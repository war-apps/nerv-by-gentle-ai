package migrate

import (
	"io/fs"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"gopkg.in/yaml.v3"
)

// RemovedProviders are the task providers nerv no longer ships.
var RemovedProviders = []string{"jira", "github-projects"}

// teamwork is the 4.2.0 default task provider, and the only one that had
// tasks.rounding_minutes.
const teamwork = "teamwork"

// mapping returns the mapping node at a dotted path of a parsed document
// (nil when absent or not a mapping).
func mapping(root *yaml.Node, path string) *yaml.Node {
	n := root
	if n != nil && n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if path == "" {
		if n != nil && n.Kind == yaml.MappingNode {
			return n
		}
		return nil
	}
	for _, seg := range strings.Split(path, ".") {
		if n == nil || n.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == seg {
				next = n.Content[i+1]
			}
		}
		n = next
	}
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	return n
}

// keys are a mapping node's keys, in order.
func keys(m *yaml.Node) []string {
	if m == nil {
		return nil
	}
	var out []string
	for i := 0; i+1 < len(m.Content); i += 2 {
		out = append(out, m.Content[i].Value)
	}
	return out
}

// value is a mapping node's value of key (nil when absent).
func value(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func parseYAML(text string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// migrateTasks applies the task rules to doc (the file at where):
// tasks.rounding_minutes moves to the teamwork provider, the removed
// providers (jira, github-projects) are dropped, and teamwork's
// task_ref_prefix (fixed by its manifest now) is dropped.
func migrateTasks(p *Plan, doc *config.Document, where string) error {
	root, err := parseYAML(doc.String())
	if err != nil {
		return refuse("%s is not valid YAML: %v", where, err)
	}
	if v, ok := doc.Scalar("tasks.provider"); ok && slices.Contains(RemovedProviders, v) {
		doc.Remove("tasks.provider")
		p.warn("%s: tasks.provider: %s was removed from nerv (dropped; with no provider the task extension uses the only installed one)", where, v)
	}
	for _, name := range RemovedProviders {
		if doc.Remove("tasks.providers." + name) {
			p.warn("%s: tasks.providers.%s: the %s provider was removed from nerv (dropped: %s)", where, name, name, strings.Join(keys(mapping(root, "tasks.providers."+name)), ", "))
		}
	}
	if v, ok := doc.Scalar("tasks.rounding_minutes"); ok {
		target := "tasks.providers." + teamwork + ".rounding_minutes"
		if existing, set := doc.Scalar(target); set {
			p.warn("%s: tasks.rounding_minutes: %s dropped, %s is already %s", where, v, target, existing)
		} else {
			doc.SetScalar(target, v)
			p.note("%s: tasks.rounding_minutes moved to %s", where, target)
		}
		doc.Remove("tasks.rounding_minutes")
	}
	if _, ok := doc.Scalar("tasks.providers." + teamwork + ".task_ref_prefix"); ok {
		doc.Remove("tasks.providers." + teamwork + ".task_ref_prefix")
		p.note("%s: tasks.providers.teamwork.task_ref_prefix dropped: the prefix is fixed by the provider's manifest", where)
	}
	return nil
}

// personalKeys are the keys of the project file that belong to the user:
// models, and every task provider key whose manifest scope is user. With
// user they move to the user configuration (unless it already sets them);
// otherwise they are reported.
func personalKeys(p *Plan, doc *config.Document, where string, fsys fs.FS, user *userState) error {
	root, err := parseYAML(doc.String())
	if err != nil {
		return refuse("%s is not valid YAML: %v", where, err)
	}
	if block, ok := doc.Block("models"); ok && value(mapping(root, ""), "models") != nil {
		switch {
		case user == nil:
			p.warn("%s: models: personal settings in the project file; run nerv migrate --user here to move them to the user config", where)
		case user.doc.KeyExists("models"):
			p.warn("%s: models: kept in the project file, the user config has its own models: block (merge them by hand)", where)
		default:
			user.doc.SetBlock("models", block)
			doc.Remove("models")
			p.note("%s: models moved to the user config %s", where, user.path)
		}
	}
	providers, err := configure.EmbeddedProviders(fsys)
	if err != nil {
		return err
	}
	for _, pr := range providers {
		name := pr.Manifest.Name
		section := mapping(root, "tasks.providers."+name)
		for _, key := range keys(section) {
			path := configure.ProviderKeyPath(name, key)
			c, known := pr.Key(key)
			if !known && key != "enabled" {
				p.warn("%s: %s is not a setting of the %s provider (kept; nerv ignores it)", where, path, name)
				continue
			}
			if !known || c.Scope != "user" {
				continue
			}
			v, scalar := doc.Scalar(path)
			switch {
			case user == nil:
				p.warn("%s: %s: a personal setting in the project file; run nerv migrate --user here to move it to the user config", where, path)
			case !scalar:
				p.warn("%s: %s: a personal setting in the project file; move it to the user config %s by hand", where, path, user.path)
			default:
				if existing, set := user.doc.Scalar(path); set {
					p.warn("%s: %s: kept in the project file, the user config already sets it to %s", where, path, existing)
					continue
				}
				var opts []config.ScalarOption
				if strings.HasPrefix(v, "{") || strings.HasPrefix(v, "[") {
					opts = append(opts, config.Raw())
				}
				user.doc.SetScalar(path, v, opts...)
				doc.Remove(path)
				p.note("%s: %s moved to the user config %s", where, path, user.path)
			}
		}
	}
	return nil
}
