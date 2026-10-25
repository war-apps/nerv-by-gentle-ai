package releaseconfig

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig/yamlsection"
	"gopkg.in/yaml.v3"
)

// EngineConflict is one engine key ImportSection would change: the
// section already sets it to Current, and the import gives Imported (both
// rendered as one-line YAML).
type EngineConflict struct {
	Key, Current, Imported string
}

// EngineConflictError is ImportSection's refusal to replace engine keys the
// section already sets to other values, without force. Section is the
// keys' path (release.providers.conventional-commits).
type EngineConflictError struct {
	Section   string
	Conflicts []EngineConflict
}

func (e *EngineConflictError) Error() string {
	parts := make([]string, len(e.Conflicts))
	for i, c := range e.Conflicts {
		parts[i] = fmt.Sprintf("%s.%s is %s, the import gives %s", e.Section, c.Key, c.Current, c.Imported)
	}
	return fmt.Sprintf("%s already sets %d engine key(s) to other values: %s", e.Section, len(parts), strings.Join(parts, "; "))
}

// ImportSection returns the .ai-workflow/config.yaml text data with imported (the
// YAML mapping a version engine's import answered, such as an imported
// semantic-release configuration) written under
// release.providers.<provider>: each key it sets is written, and every
// other key (of the section, of the provider, the projects and their
// builds) is kept, with its comments. A key the provider's block already
// sets to another value is never replaced silently: without force the
// result is an *EngineConflictError listing each one; with force it is
// replaced in place and notes names the replaced value. A 4.2.0 release:
// section is not converted here: the result does not load, and the refusal
// names nerv migrate. The rest of the file is kept verbatim
// (internal/config's line editor). The result must load: an invalid one is
// an error, and nothing should be written.
func ImportSection(data []byte, provider, imported string, force bool) (string, []string, error) {
	var notes []string
	doc := config.Parse(string(data))
	section, err := yamlsection.Locate(data, SectionKey)
	if err != nil && !errors.Is(err, yamlsection.ErrAbsent) {
		return "", nil, err
	}
	if section == nil || section.ShortTag() == "!!null" {
		section = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if section.Kind != yaml.MappingNode {
		return "", nil, &ValidationError{Problems: []string{SectionKey + ": must be a mapping"}}
	}
	var values yaml.Node
	if err := yaml.Unmarshal([]byte(imported), &values); err != nil {
		return "", nil, fmt.Errorf("the imported keys: %w", err)
	}
	importedMap := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if len(values.Content) == 1 && values.Content[0].Kind == yaml.MappingNode {
		importedMap = values.Content[0]
	}
	at := SectionKey + ".providers." + provider
	providers := mappingValue(section, "providers")
	if providers == nil || providers.ShortTag() == "!!null" {
		if providers == nil {
			providers = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			section.Content = append([]*yaml.Node{scalarNode("providers"), providers}, section.Content...)
		} else {
			*providers = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
	}
	if providers.Kind != yaml.MappingNode {
		return "", nil, &ValidationError{Problems: []string{SectionKey + ".providers: must be a mapping"}}
	}
	block := mappingValue(providers, provider)
	if block == nil || block.ShortTag() == "!!null" {
		if block == nil {
			block = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			providers.Content = append([]*yaml.Node{scalarNode(provider), block}, providers.Content...)
		} else {
			*block = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
	}
	if block.Kind != yaml.MappingNode {
		return "", nil, &ValidationError{Problems: []string{at + ": must be a mapping"}}
	}

	var conflicts []EngineConflict
	existing := map[string]bool{}
	for i := 0; i+1 < len(block.Content); i += 2 {
		key := block.Content[i].Value
		v := mappingValue(importedMap, key)
		if v == nil {
			continue
		}
		existing[key] = true
		cur := block.Content[i+1]
		if sameValue(cur, v) {
			continue
		}
		conflicts = append(conflicts, EngineConflict{Key: key, Current: oneLine(cur), Imported: oneLine(v)})
		if force {
			v.HeadComment, v.LineComment, v.FootComment = cur.HeadComment, cur.LineComment, cur.FootComment
			block.Content[i+1] = v
		}
	}
	if len(conflicts) > 0 && !force {
		return "", nil, &EngineConflictError{Section: at, Conflicts: conflicts}
	}
	for _, c := range conflicts {
		notes = append(notes, fmt.Sprintf("replaced %s.%s: %s (now %s)", at, c.Key, c.Current, c.Imported))
	}
	var added []*yaml.Node
	for i := 0; i+1 < len(importedMap.Content); i += 2 {
		if key := importedMap.Content[i].Value; !existing[key] {
			added = append(added, importedMap.Content[i], importedMap.Content[i+1])
		}
	}
	block.Content = append(added, block.Content...)

	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{scalarNode(SectionKey), section}}
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

func scalarNode(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
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
