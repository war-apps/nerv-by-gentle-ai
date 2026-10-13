package shipconfig

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

var typeOfRelease = reflect.TypeFor[Release]()

// maxWalkNodes bounds the unknown-field walk. Aliases and merge keys are
// expanded at every use, so a few lines of YAML can describe billions of
// nodes; a real release section visits a few hundred.
const maxWalkNodes = 10000

// unknownFields walks the release: node against the yaml tags of Release
// and reports every mapping key that no struct field declares, with its
// path and line. yaml.v3 offers KnownFields only on a Decoder over a whole
// document, and the rest of nerv.yaml must stay free-form, so the section
// is checked here instead. Merge keys (<<: *anchor, <<: [*a, *b]) are
// followed into the merged mappings. Kind mismatches are left to yaml's
// own decoding. It fails when the walk exceeds maxWalkNodes.
func unknownFields(node *yaml.Node) ([]string, error) {
	w := walker{}
	problems := w.walk(node, typeOfRelease, "release")
	if w.visited > maxWalkNodes {
		return nil, fmt.Errorf("release: the section expands to more than %d nodes through aliases and merge keys", maxWalkNodes)
	}
	return problems, nil
}

type walker struct{ visited int }

func (w *walker) walk(node *yaml.Node, t reflect.Type, path string) []string {
	if w.visited++; w.visited > maxWalkNodes {
		return nil
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		node = node.Alias
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	var problems []string
	switch t.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return nil
		}
		fields := yamlFields(t)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if isMergeKey(key) {
				for _, m := range mergedMappings(value) {
					problems = append(problems, w.walk(m, t, path)...)
				}
				continue
			}
			ft, ok := fields[key.Value]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s.%s: unknown field (line %d)", path, key.Value, key.Line))
				continue
			}
			problems = append(problems, w.walk(value, ft, path+"."+key.Value)...)
		}
	case reflect.Slice:
		if node.Kind != yaml.SequenceNode {
			return nil
		}
		for i, item := range node.Content {
			problems = append(problems, w.walk(item, t.Elem(), fmt.Sprintf("%s[%d]", path, i))...)
		}
	case reflect.Map:
		if node.Kind != yaml.MappingNode {
			return nil
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if isMergeKey(key) {
				for _, m := range mergedMappings(value) {
					problems = append(problems, w.walk(m, t, path)...)
				}
				continue
			}
			problems = append(problems, w.walk(value, t.Elem(), path+"."+key.Value)...)
		}
	}
	return problems
}

// isMergeKey reports whether key is the YAML merge key <<, as yaml.v3
// recognises it.
func isMergeKey(key *yaml.Node) bool {
	return key.Kind == yaml.ScalarNode && key.Value == "<<" &&
		(key.Tag == "" || key.Tag == "!" || key.ShortTag() == "!!merge")
}

// mergedMappings returns the mappings a merge key's value names: one
// mapping, or a sequence of them, each possibly through an alias. Anything
// else is left to yaml's own decoding error.
func mergedMappings(value *yaml.Node) []*yaml.Node {
	resolve := func(n *yaml.Node) *yaml.Node {
		if n.Kind == yaml.AliasNode && n.Alias != nil {
			return n.Alias
		}
		return n
	}
	value = resolve(value)
	switch value.Kind {
	case yaml.MappingNode:
		return []*yaml.Node{value}
	case yaml.SequenceNode:
		var out []*yaml.Node
		for _, item := range value.Content {
			if item = resolve(item); item.Kind == yaml.MappingNode {
				out = append(out, item)
			}
		}
		return out
	}
	return nil
}

// yamlFields maps each yaml key of struct type t to its field type.
func yamlFields(t reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}
		fields[name] = f.Type
	}
	return fields
}
