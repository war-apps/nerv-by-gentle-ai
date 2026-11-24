package workflowconfig

import (
	"fmt"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"gopkg.in/yaml.v3"
)

// Sections are the sections of the extensions nerv used to ship (task,
// release and deploy), now read by the NERV-ecosystem tools from their own
// files: nerv keeps them only so nerv migrate can move them there. They
// are never read from the user file, except their personal keys
// (Personal).
var Sections = []string{"tasks", "release", "deploy"}

// DefaultsKey is the user file's section of seeds: defaults.<section> has
// the shape of the project's <section> and never applies at run time;
// nerv migrate moves defaults.tasks, defaults.release and defaults.deploy
// to the tools' own files.
const DefaultsKey = "defaults"

// PersonalTaskKeys are nerv-task's own machine-local keys (frozen from
// the task extension nerv used to ship): a
// user's timer store and staleness threshold, and the older
// tasks.rounding_minutes, which a user-scope provider key replaces. They
// stay readable from the user file.
var PersonalTaskKeys = []string{"tasks.timer_store", "tasks.stale_after_hours", "tasks.rounding_minutes"}

// PersonalProviderKeys are the scope: user keys of the providers nerv
// shipped (task-teamwork and release-ssh-compose), frozen as their last
// manifests declared them: nerv migrate moves them out of nerv's files
// without reading any manifest.
var PersonalProviderKeys = []string{
	"tasks.providers.teamwork.assignee_id",
	"tasks.providers.teamwork.default_project_id",
	"tasks.providers.teamwork.default_tasklist_id",
	"tasks.providers.teamwork.rounding_minutes",
	"tasks.providers.teamwork.billable",
	"tasks.providers.teamwork.stages",
	"tasks.providers.teamwork.stage_variants",
	"tasks.providers.teamwork.comment_labels",
	"tasks.providers.teamwork.comment_rest",
	"tasks.providers.teamwork.known_projects",
	"deploy.providers.ssh-compose.infisical_domain",
	"deploy.providers.ssh-compose.ssh_auth",
	"deploy.providers.ssh-compose.ssh_key_file",
	"deploy.providers.ssh-compose.ssh_known_hosts",
	"deploy.providers.ssh-compose.ssh_connect_timeout",
}

// Personal is the set of personal keys under Sections, frozen:
// PersonalTaskKeys and PersonalProviderKeys.
type Personal struct{}

// Has reports whether path, or a key it is under, is personal.
func (p Personal) Has(path string) bool {
	for k := path; k != ""; {
		if slices.Contains(PersonalTaskKeys, k) || slices.Contains(PersonalProviderKeys, k) {
			return true
		}
		i := strings.LastIndex(k, ".")
		if i < 0 {
			break
		}
		k = k[:i]
	}
	return false
}

// ProjectOnly reports whether path is under one of Sections.
func ProjectOnly(path string) bool {
	section, _, _ := strings.Cut(path, ".")
	return slices.Contains(Sections, section)
}

// RunTimeUserKeys lists the keys of a user document under Sections that
// are not personal, so no longer apply at run time: each child of a
// section (tasks.provider, release.build), and each key of a provider
// under <section>.providers.<provider>. defaults: is not listed.
func RunTimeUserKeys(doc *config.Document, personal Personal) ([]string, error) {
	var out []string
	for _, section := range Sections {
		keys, err := keysOf(doc, section, func(k string) bool { return !personal.Has(k) })
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
	}
	return out, nil
}

// Defaults is the user document's defaults: section as a document of its
// own (its keys at column 0), or nil when it has none.
func Defaults(user *config.Document) *config.Document {
	if user == nil {
		return nil
	}
	block, found := user.Block(DefaultsKey)
	if !found {
		return nil
	}
	lines := strings.Split(block, "\n")[1:]
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, "  ")
	}
	return config.Parse(strings.Join(lines, "\n"))
}

// Strip removes from doc's <section> every key drop reports (a section
// child, or a provider key, as RunTimeUserKeys names them), then the
// provider and providers keys it leaves empty, then the section itself
// when it is left empty.
func Strip(doc *config.Document, section string, drop func(path string) bool) error {
	paths, err := keysOf(doc, section, drop)
	if err != nil || len(paths) == 0 {
		return err
	}
	for _, p := range paths {
		doc.Remove(p)
	}
	root, err := parseRoot(doc.String())
	if err != nil {
		return err
	}
	eachPair(child(child(root, section), "providers"), func(provider string, keys *yaml.Node) {
		if empty(keys) {
			doc.Remove(section + ".providers." + provider)
		}
	})
	for _, path := range []string{section + ".providers", section} {
		if root, err = parseRoot(doc.String()); err != nil {
			return err
		}
		node := root
		for _, seg := range strings.Split(path, ".") {
			node = child(node, seg)
		}
		if node != nil && empty(node) {
			doc.Remove(path)
		}
	}
	return nil
}

// sectionKeys lists the keys of one section node, as RunTimeUserKeys
// names them: each child of the section, except providers, whose
// provider keys are listed instead (<section>.providers.<provider>.<key>).
func sectionKeys(section *yaml.Node, name string) []string {
	var out []string
	eachPair(section, func(key string, value *yaml.Node) {
		path := name + "." + key
		if key != "providers" || value.Kind != yaml.MappingNode {
			out = append(out, path)
			return
		}
		eachPair(value, func(provider string, keys *yaml.Node) {
			eachPair(keys, func(k string, _ *yaml.Node) {
				out = append(out, path+"."+provider+"."+k)
			})
		})
	})
	return out
}

// keysOf lists the keys of doc's <section> (sectionKeys) that keep reports.
func keysOf(doc *config.Document, section string, keep func(string) bool) ([]string, error) {
	root, err := parseRoot(doc.String())
	if err != nil {
		return nil, err
	}
	var out []string
	for _, k := range sectionKeys(child(root, section), section) {
		if keep(k) {
			out = append(out, k)
		}
	}
	return out, nil
}

func parseRoot(text string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("not valid YAML: %w", err)
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0], nil
	}
	return nil, nil
}

// child is a mapping node's value of key (nil when absent).
func child(m *yaml.Node, key string) *yaml.Node {
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

// eachPair calls f with each key and value of a mapping node, in order.
func eachPair(m *yaml.Node, f func(key string, value *yaml.Node)) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		f(m.Content[i].Value, m.Content[i+1])
	}
}

// empty reports whether a node holds nothing: a null, or an empty mapping.
func empty(n *yaml.Node) bool {
	return (n.Kind == yaml.ScalarNode && n.Tag == "!!null") || (n.Kind == yaml.MappingNode && len(n.Content) == 0)
}
