package workflowconfig

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
	"gopkg.in/yaml.v3"
)

// Sections are the extensions' sections (the task, release and deploy
// extensions). At run time they are read from the project file only,
// except their personal keys (Personal), which keep being read from the
// user file.
var Sections = []string{"tasks", "release", "deploy"}

// DefaultsKey is the user file's section of seeds: defaults.<section> has
// the shape of the project's <section>, never applies at run time, and is
// copied into a project by the flow that writes that section there
// (/nerv:init and nerv configure --init-repo for tasks:, /release:setup,
// /deploy:setup) when the project lacks the key.
const DefaultsKey = "defaults"

// PersonalTaskKeys are the task extension's own machine-local keys: a
// user's timer store and staleness threshold, and the older
// tasks.rounding_minutes, which a user-scope provider key replaces. They
// stay readable from the user file.
var PersonalTaskKeys = []string{"tasks.timer_store", "tasks.stale_after_hours", "tasks.rounding_minutes"}

// Personal is the set of personal keys under Sections: PersonalTaskKeys
// and every provider config key whose manifest declares scope: user, at
// <section>.providers.<provider>.<key>. The zero value holds only
// PersonalTaskKeys.
type Personal struct {
	keys map[string]bool
}

// LoadPersonal reads the user-scope provider keys from the provider
// manifests of fsys (typically nerv.ExtensionsFS(), or extension.WithProject
// over it); a nil fsys has none.
func LoadPersonal(fsys fs.FS) (Personal, error) {
	p := Personal{keys: map[string]bool{}}
	if fsys == nil {
		return p, nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return Personal{}, fmt.Errorf("listing extensions and providers: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := fs.Stat(fsys, e.Name()+"/"+extension.ProviderFile); err != nil {
			continue
		}
		prov, _, err := extension.LoadProvider(fsys, e.Name())
		if err != nil {
			return Personal{}, err
		}
		for _, c := range prov.Config {
			if c.Scope == "user" {
				p.keys[SectionOf(prov.Extension)+".providers."+prov.Name+"."+c.Key] = true
			}
		}
	}
	return p, nil
}

// SectionOf is the configuration section of an extension: tasks for the
// task extension, the extension's name otherwise.
func SectionOf(extensionName string) string {
	if extensionName == plugin.TaskPlugin {
		return "tasks"
	}
	return extensionName
}

// Has reports whether path, or a key it is under, is personal.
func (p Personal) Has(path string) bool {
	for k := path; k != ""; {
		if slices.Contains(PersonalTaskKeys, k) || p.keys[k] {
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

// PersonalKeysOf lists the personal keys doc's <section> holds.
func PersonalKeysOf(doc *config.Document, section string, personal Personal) ([]string, error) {
	return keysOf(doc, section, personal.Has)
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

// Seed is the user's defaults.<section> as a top-level <section>: block for
// a project, without its personal keys (they never go into a committed
// file). ok is false when there is no seed, or nothing but personal keys.
func Seed(user *config.Document, section string, personal Personal) (block string, ok bool) {
	defaults := Defaults(user)
	if defaults == nil {
		return "", false
	}
	text, found := defaults.Block(section)
	if !found {
		return "", false
	}
	doc := config.Parse(strings.TrimRight(text, "\n") + "\n")
	if err := Strip(doc, section, personal.Has); err != nil {
		return "", false
	}
	if text, found = doc.Block(section); !found {
		return "", false
	}
	return strings.TrimRight(text, "\n") + "\n", true
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
