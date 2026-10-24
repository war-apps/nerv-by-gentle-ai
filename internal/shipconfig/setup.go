package shipconfig

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig/yamlsection"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
	"gopkg.in/yaml.v3"
)

// engineKeys are the ship: section's version engine keys, in the order
// ImportSection writes them, before the projects.
var engineKeys = []string{"branches", "release_rules", "note_keywords", "tag_format", "repository_url", "changelog"}

// EngineConflict is one engine key ImportSection would change: the
// section already sets it to Current, and the import gives Imported (both
// rendered as one-line YAML).
type EngineConflict struct {
	Key, Current, Imported string
}

// EngineConflictError is ImportSection's refusal to replace engine keys the
// section already sets to other values, without force.
type EngineConflictError struct {
	Section   string
	Conflicts []EngineConflict
}

func (e *EngineConflictError) Error() string {
	parts := make([]string, len(e.Conflicts))
	for i, c := range e.Conflicts {
		parts[i] = fmt.Sprintf("%s.%s is %s, the import gives %s", e.Section, c.Key, c.Current, c.Imported)
	}
	return fmt.Sprintf("the %s: section already sets %d engine key(s) to other values: %s", e.Section, len(parts), strings.Join(parts, "; "))
}

// ImportSection returns the .ai-workflow.yml text data with the version
// engine settings of cfg (an imported semantic-release configuration)
// written into its ship: section: each engine key cfg sets is written, and
// every other key of the section (the projects, their builds, the engine
// keys cfg leaves unset) is kept, with its comments. An engine key the
// section already sets to another value is never replaced silently:
// without force the result is an *EngineConflictError listing each one;
// with force it is replaced in place and notes names the replaced value.
// A file with the old release: section and no ship: section gets it
// renamed to ship: (semantic_release: dropped when it was there), and notes
// says so. The rest of the file is kept verbatim (internal/config's line
// editor). The result must load: an invalid one is an error, and nothing
// should be written.
func ImportSection(data []byte, cfg versioning.Config, force bool) (string, []string, error) {
	var notes []string
	doc := config.Parse(string(data))
	name := SectionKey
	section, err := yamlsection.Locate(data, SectionKey)
	switch {
	case err == nil:
	case errors.Is(err, yamlsection.ErrAbsent):
		old, oerr := yamlsection.Locate(data, "release")
		switch {
		case oerr == nil:
			name, section = "release", old
		case !errors.Is(oerr, yamlsection.ErrAbsent):
			return "", nil, fmt.Errorf("release: %w", oerr)
		}
	default:
		return "", nil, err
	}
	if section == nil || section.ShortTag() == "!!null" {
		section = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if section.Kind != yaml.MappingNode {
		return "", nil, &ValidationError{Problems: []string{name + ": must be a mapping"}}
	}
	if name == "release" {
		note := "the release: section was renamed to ship:"
		if len(takeRemovedSectionKeys(section)) > 0 {
			note += " (semantic_release: dropped: the version engine is native)"
		}
		notes = append(notes, note)
	}
	values, err := engineValues(cfg)
	if err != nil {
		return "", nil, err
	}
	var conflicts []EngineConflict
	existing := map[string]bool{}
	for i := 0; i+1 < len(section.Content); i += 2 {
		key := section.Content[i].Value
		v, engine := values[key]
		if !engine {
			continue
		}
		existing[key] = true
		cur := section.Content[i+1]
		if sameValue(cur, v) {
			continue
		}
		conflicts = append(conflicts, EngineConflict{Key: key, Current: oneLine(cur), Imported: oneLine(v)})
		if force {
			v.HeadComment, v.LineComment, v.FootComment = cur.HeadComment, cur.LineComment, cur.FootComment
			section.Content[i+1] = v
		}
	}
	if len(conflicts) > 0 && !force {
		return "", nil, &EngineConflictError{Section: name, Conflicts: conflicts}
	}
	for _, c := range conflicts {
		notes = append(notes, fmt.Sprintf("replaced %s.%s: %s (now %s)", name, c.Key, c.Current, c.Imported))
	}
	var added []*yaml.Node
	for _, key := range engineKeys {
		if v, ok := values[key]; ok && !existing[key] {
			added = append(added, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
		}
	}
	section.Content = append(added, section.Content...)

	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: SectionKey}, section}}
	if err := enc.Encode(root); err != nil {
		return "", nil, err
	}
	if err := enc.Close(); err != nil {
		return "", nil, err
	}
	if name == "release" {
		doc.SetBlock("release", "")
	}
	doc.SetBlock(SectionKey, strings.TrimRight(b.String(), "\n"))
	out := doc.String()
	if _, err := parse([]byte(out)); err != nil && !errors.Is(err, ErrNotConfigured) {
		return "", nil, err
	}
	return out, notes, nil
}

// sameValue reports whether two YAML nodes hold the same data, whatever
// their style.
func sameValue(a, b *yaml.Node) bool {
	var av, bv any
	if a.Decode(&av) != nil || b.Decode(&bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// oneLine renders a node as one line of flow-style YAML, keys in their
// written order, without comments, for messages.
func oneLine(n *yaml.Node) string {
	if n.Kind == yaml.ScalarNode {
		return n.Value
	}
	out, err := yaml.Marshal(flowCopy(n))
	if err != nil {
		return "(unreadable)"
	}
	return strings.TrimSpace(string(out))
}

// flowCopy is a deep copy of n in flow style, without comments.
func flowCopy(n *yaml.Node) *yaml.Node {
	c := *n
	c.HeadComment, c.LineComment, c.FootComment = "", "", ""
	if c.Kind == yaml.MappingNode || c.Kind == yaml.SequenceNode {
		c.Style = yaml.FlowStyle
	}
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, child := range n.Content {
		c.Content[i] = flowCopy(child)
	}
	return &c
}

// engineValues are cfg's engine settings as YAML nodes, keyed by their
// section key; empty settings are left out.
func engineValues(cfg versioning.Config) (map[string]*yaml.Node, error) {
	values := map[string]*yaml.Node{}
	set := func(key string, v any) error {
		var n yaml.Node
		if err := n.Encode(v); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		values[key] = &n
		return nil
	}
	var branches []any
	for _, br := range cfg.Branches {
		if !br.Prerelease.IsSet() && br.Channel == "" {
			branches = append(branches, br.Name)
			continue
		}
		m := yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
		add := func(k string, v any) {
			var n yaml.Node
			_ = n.Encode(v)
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &n)
		}
		add("name", br.Name)
		switch {
		case br.Prerelease.UsesName():
			add("prerelease", true)
		case br.Prerelease.ID() != "":
			add("prerelease", br.Prerelease.ID())
		}
		if br.Channel != "" {
			add("channel", br.Channel)
		}
		branches = append(branches, &m)
	}
	if len(branches) > 0 {
		if err := set("branches", branches); err != nil {
			return nil, err
		}
	}
	if len(cfg.ReleaseRules) > 0 {
		var rules []any
		for _, r := range cfg.ReleaseRules {
			m := yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
			add := func(k string, v any) {
				var n yaml.Node
				_ = n.Encode(v)
				m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &n)
			}
			if r.Type != "" {
				add("type", r.Type)
			}
			if r.Scope != "" {
				add("scope", r.Scope)
			}
			if r.Breaking {
				add("breaking", true)
			}
			if r.Revert {
				add("revert", true)
			}
			add("release", r.Release)
			rules = append(rules, &m)
		}
		if err := set("release_rules", rules); err != nil {
			return nil, err
		}
	}
	if len(cfg.NoteKeywords) > 0 {
		if err := set("note_keywords", cfg.NoteKeywords); err != nil {
			return nil, err
		}
	}
	for key, v := range map[string]string{"tag_format": cfg.TagFormat, "repository_url": cfg.RepositoryURL} {
		if v != "" {
			if err := set(key, v); err != nil {
				return nil, err
			}
		}
	}
	changelog := map[string]string{}
	if cfg.Changelog.File != "" {
		changelog["file"] = cfg.Changelog.File
	}
	if cfg.Changelog.Title != "" {
		changelog["title"] = cfg.Changelog.Title
	}
	if len(changelog) > 0 {
		if err := set("changelog", changelog); err != nil {
			return nil, err
		}
	}
	return values, nil
}
