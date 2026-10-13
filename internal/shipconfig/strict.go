package shipconfig

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

var typeOfRelease = reflect.TypeFor[Release]()

// unknownFields walks node against the yaml tags of t and reports every
// mapping key that no struct field declares, with its path and line.
// yaml.v3 offers KnownFields only on a Decoder over a whole document, and
// the rest of nerv.yaml must stay free-form, so the release: subtree is
// checked here instead. Kind mismatches are left to yaml's own decoding.
func unknownFields(node *yaml.Node, t reflect.Type, path string) []string {
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
			ft, ok := fields[key.Value]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s.%s: unknown field (line %d)", path, key.Value, key.Line))
				continue
			}
			problems = append(problems, unknownFields(value, ft, path+"."+key.Value)...)
		}
	case reflect.Slice:
		if node.Kind != yaml.SequenceNode {
			return nil
		}
		for i, item := range node.Content {
			problems = append(problems, unknownFields(item, t.Elem(), fmt.Sprintf("%s[%d]", path, i))...)
		}
	case reflect.Map:
		if node.Kind != yaml.MappingNode {
			return nil
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			problems = append(problems, unknownFields(node.Content[i+1], t.Elem(), path+"."+node.Content[i].Value)...)
		}
	}
	return problems
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
