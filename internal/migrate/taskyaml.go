package migrate

import (
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
	"gopkg.in/yaml.v3"
)

// moveRepoTaskYAML moves doc's tasks: section into the repository's own
// task.yaml (o.TaskYAMLRepo, nerv-task's repository configuration file),
// key by key, without the "tasks." prefix: nerv-task reads its own
// repository keys from there now, not from doc. A personal (scope: user)
// key is left for the existing personalKeys check to report; it is never
// swept into the repository's task.yaml. A key already set in task.yaml
// is kept in doc and reported as a conflict; every other key is moved
// there and removed from doc. A no-op when o.TaskYAMLRepo is "".
func moveRepoTaskYAML(p *Plan, o Options, doc *config.Document) error {
	if o.TaskYAMLRepo == "" {
		return nil
	}
	personal, err := workflowconfig.LoadPersonal(o.ExtensionsFS)
	if err != nil {
		return err
	}
	skip := func(leaf string) bool { return personal.Has("tasks." + leaf) }

	existing, exists, err := readOptional(o.TaskYAMLRepo)
	if err != nil {
		return err
	}
	taskDoc := config.Parse(string(existing))
	migrated, err := migrateSection(p, doc, "tasks", taskDoc, "", paths.ProjectConfigFile, "the repository task.yaml", skip)
	if err != nil {
		return err
	}
	if len(migrated) == 0 {
		return nil
	}
	removeMigrated(doc, "tasks", migrated)
	text := taskDoc.String()
	if err := verifyTaskYAML(o.TaskYAMLRepo, text); err != nil {
		return err
	}
	c := Change{Kind: Write, Path: o.TaskYAMLRepo, New: []byte(text), Why: "nerv-task's repository configuration, moved from " + paths.ProjectConfigFile}
	if exists {
		c.Old = existing
	}
	p.Changes = append(p.Changes, c)
	p.note("%s: %s moved to %s", paths.ProjectConfigFile, strings.Join(migrated, ", "), o.TaskYAMLRepo)
	return nil
}

// moveUserTaskYAML moves the user config's tasks: personal keys and
// defaults.tasks: seed into the global task.yaml (o.TaskYAMLHome,
// nerv-task's global configuration file): the former as task.yaml's own
// top-level keys, the latter under task.yaml's defaults: (both without
// the "tasks."/"defaults.tasks." prefix). nerv-task reads its own global
// keys from there now, not from the user config. A key already set in
// task.yaml is kept in the user config and reported as a conflict; every
// other key is moved there and removed from the user config. A no-op
// when o.TaskYAMLHome is "".
func moveUserTaskYAML(p *Plan, o Options, u *userState) error {
	if o.TaskYAMLHome == "" {
		return nil
	}
	existing, exists, err := readOptional(o.TaskYAMLHome)
	if err != nil {
		return err
	}
	taskDoc := config.Parse(string(existing))
	migratedTasks, err := migrateSection(p, u.doc, "tasks", taskDoc, "", "user config", "the global task.yaml", nil)
	if err != nil {
		return err
	}
	migratedDefaults, err := migrateSection(p, u.doc, "defaults.tasks", taskDoc, "defaults", "user config", "the global task.yaml", nil)
	if err != nil {
		return err
	}
	if len(migratedTasks) == 0 && len(migratedDefaults) == 0 {
		return nil
	}
	removeMigrated(u.doc, "tasks", migratedTasks)
	removeMigrated(u.doc, "defaults.tasks", migratedDefaults)
	text := taskDoc.String()
	if err := verifyTaskYAML(o.TaskYAMLHome, text); err != nil {
		return err
	}
	c := Change{Kind: Write, Path: o.TaskYAMLHome, New: []byte(text), User: true, Why: "nerv-task's global configuration, moved from your user configuration"}
	if exists {
		c.Old = existing
	}
	p.Changes = append(p.Changes, c)
	if len(migratedTasks) > 0 {
		p.note("user config: %s moved to %s", strings.Join(migratedTasks, ", "), o.TaskYAMLHome)
	}
	if len(migratedDefaults) > 0 {
		p.note("user config: %s moved to %s under defaults:", strings.Join(migratedDefaults, ", "), o.TaskYAMLHome)
	}
	return nil
}

// migrateSection moves every leaf of src's srcSection (see sectionLeaves)
// into dst, each as "<dstPrefix>.<leaf>" (dstPrefix "" for dst's own
// root): a scalar leaf verbatim, a nested map or list inlined (flow
// style, so its types and nesting survive). skip, when non-nil, excludes
// a leaf (such as a personal key that belongs in the user config
// instead) from the move entirely; it is neither moved nor reported here.
// A leaf already set in dst is left in src and reported as a conflict
// (where names src, target names dst). It reports the srcSection-relative
// paths it moved (such as "tasks.timer_store"), for the caller to remove
// from src once dst's write is verified.
func migrateSection(p *Plan, src *config.Document, srcSection string, dst *config.Document, dstPrefix, where, target string, skip func(leaf string) bool) ([]string, error) {
	srcRoot, err := parseYAML(src.String())
	if err != nil {
		return nil, refuse("%s is not valid YAML: %v", where, err)
	}
	dstRoot, err := parseYAML(dst.String())
	if err != nil {
		return nil, refuse("%s is not valid YAML: %v", target, err)
	}
	var migrated []string
	for _, leaf := range sectionLeaves(srcRoot, srcSection) {
		if skip != nil && skip(leaf) {
			continue
		}
		dstPath := leaf
		if dstPrefix != "" {
			dstPath = dstPrefix + "." + leaf
		}
		if existing := nodeAt(dstRoot, dstPath); existing != nil {
			p.warn("%s: %s.%s: kept, %s already sets %s to %s", where, srcSection, leaf, target, dstPath, renderValue(existing))
			continue
		}
		n := nodeAt(srcRoot, srcSection+"."+leaf)
		if n == nil {
			continue
		}
		value, raw := leafValue(n)
		var opts []config.ScalarOption
		if raw {
			opts = append(opts, config.Raw())
		}
		dst.SetScalar(dstPath, value, opts...)
		migrated = append(migrated, srcSection+"."+leaf)
	}
	return migrated, nil
}

// sectionLeaves lists the leaf paths under root's section (a dotted path
// into root, such as "tasks" or "defaults.tasks"): each direct child of
// the section, except "providers", whose keys are listed as
// "providers.<provider>.<key>" instead.
func sectionLeaves(root *yaml.Node, section string) []string {
	sec := mapping(root, section)
	if sec == nil {
		return nil
	}
	var out []string
	for i := 0; i+1 < len(sec.Content); i += 2 {
		key := sec.Content[i].Value
		if key == "providers" && sec.Content[i+1].Kind == yaml.MappingNode {
			providers := sec.Content[i+1]
			for j := 0; j+1 < len(providers.Content); j += 2 {
				name := providers.Content[j].Value
				provMap := providers.Content[j+1]
				if provMap.Kind != yaml.MappingNode {
					continue
				}
				for k := 0; k+1 < len(provMap.Content); k += 2 {
					out = append(out, "providers."+name+"."+provMap.Content[k].Value)
				}
			}
			continue
		}
		out = append(out, key)
	}
	return out
}

// nodeAt returns the value node at a dotted path from root (a parsed
// document or any node), or nil when any segment is missing.
func nodeAt(root *yaml.Node, path string) *yaml.Node {
	parentPath, leaf := "", path
	if i := strings.LastIndex(path, "."); i >= 0 {
		parentPath, leaf = path[:i], path[i+1:]
	}
	parent := mapping(root, parentPath)
	return value(parent, leaf)
}

// leafValue is n's value text and whether it must be set raw (SetScalar's
// config.Raw(), for a nested map or list rendered inline): a scalar's
// value verbatim (SetScalar quotes it as needed), a mapping or sequence
// inlined in flow style (inlineYAML).
func leafValue(n *yaml.Node) (text string, raw bool) {
	switch n.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		return inlineYAML(n), true
	default:
		return n.Value, false
	}
}

// renderValue is n as text, for a warning message: leafValue's text,
// regardless of raw-ness.
func renderValue(n *yaml.Node) string {
	text, _ := leafValue(n)
	return text
}

// inlineYAML renders n as a flow-style YAML literal ("{ a: 1, b: [2, 3] }"),
// recursively, using config.FormatScalarToken for every scalar so types
// (ints, bools, bare strings) survive without being quoted unnecessarily.
func inlineYAML(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		parts := make([]string, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			parts = append(parts, n.Content[i].Value+": "+inlineYAML(n.Content[i+1]))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	case yaml.SequenceNode:
		parts := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			parts = append(parts, inlineYAML(c))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		if n.Tag == "!!null" {
			return "null"
		}
		return config.FormatScalarToken(n.Value)
	}
}

// isEmptyNode reports whether n holds nothing: nil, a null scalar, or an
// empty mapping.
func isEmptyNode(n *yaml.Node) bool {
	if n == nil {
		return true
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
		return true
	}
	return n.Kind == yaml.MappingNode && len(n.Content) == 0
}

// removeMigrated removes every path in migrated from doc, then cleans up
// what that leaves empty: a provider map under "<section>.providers", the
// "providers" map itself, and finally section (a dotted path, such as
// "tasks" or "defaults.tasks") when nothing is left in it.
func removeMigrated(doc *config.Document, section string, migrated []string) {
	if len(migrated) == 0 {
		return
	}
	for _, path := range migrated {
		doc.Remove(path)
	}
	providersPath := section + ".providers"
	if root, err := parseYAML(doc.String()); err == nil {
		if providers := mapping(root, providersPath); providers != nil {
			for _, name := range keys(providers) {
				if isEmptyNode(value(providers, name)) {
					doc.Remove(providersPath + "." + name)
				}
			}
		}
	}
	if root, err := parseYAML(doc.String()); err == nil && isEmptyNode(mapping(root, providersPath)) {
		doc.Remove(providersPath)
	}
	if root, err := parseYAML(doc.String()); err == nil && isEmptyNode(mapping(root, section)) {
		doc.Remove(section)
	}
}

// verifyTaskYAML re-parses text (a planned task.yaml) to catch any
// malformed YAML the migration produced, before it is queued as a
// change and src's matching keys are removed.
func verifyTaskYAML(path, text string) error {
	var v any
	if err := yaml.Unmarshal([]byte(text), &v); err != nil {
		return refuse("the migrated %s would not be valid YAML: %v", path, err)
	}
	return nil
}
