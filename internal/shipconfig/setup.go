package shipconfig

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig/yamlsection"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
	"gopkg.in/yaml.v3"
)

// engineKeys are the ship: section's version engine keys, in the order
// ImportSection writes them, before the projects.
var engineKeys = []string{"branches", "release_rules", "note_keywords", "tag_format", "repository_url", "changelog"}

// ImportSection returns the .ai-workflow.yml text data with the version
// engine settings of cfg (an imported semantic-release configuration)
// written into its ship: section: each engine key is set to cfg's value,
// and every other key of the section (the projects, their builds) is kept.
// A file with the old release: section and no ship: section gets it renamed
// to ship: (without semantic_release:), and notes says so. The rest of the
// file is kept verbatim (internal/config's line editor). The result must
// load: an invalid one is an error, and nothing should be written.
func ImportSection(data []byte, cfg versioning.Config) (string, []string, error) {
	var notes []string
	doc := config.Parse(string(data))
	section, err := yamlsection.Locate(data, SectionKey)
	switch {
	case err == nil:
	case errors.Is(err, yamlsection.ErrAbsent):
		if old, oerr := yamlsection.Locate(data, "release"); oerr == nil {
			section = old
			takeRemovedSectionKeys(section)
			doc.SetBlock("release", "")
			notes = append(notes, "the release: section was renamed to ship: (semantic_release: dropped: the version engine is native)")
		}
	default:
		return "", nil, err
	}
	if section == nil || section.ShortTag() == "!!null" {
		section = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if section.Kind != yaml.MappingNode {
		return "", nil, &ValidationError{Problems: []string{"ship: must be a mapping"}}
	}
	values, err := engineValues(cfg)
	if err != nil {
		return "", nil, err
	}
	var kept []*yaml.Node
	for i := 0; i+1 < len(section.Content); i += 2 {
		if _, engine := values[section.Content[i].Value]; !engine {
			kept = append(kept, section.Content[i], section.Content[i+1])
		}
	}
	var content []*yaml.Node
	for _, key := range engineKeys {
		if v, ok := values[key]; ok {
			content = append(content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
		}
	}
	section.Content = append(content, kept...)

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
	doc.SetBlock(SectionKey, strings.TrimRight(b.String(), "\n"))
	out := doc.String()
	if _, err := parse([]byte(out)); err != nil && !errors.Is(err, ErrNotConfigured) {
		return "", nil, err
	}
	return out, notes, nil
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
