package extension

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ConfigScopes are where a config key lives: the user's
// ~/.config/ai-workflow/config.yaml or the repository's .ai-workflow/config.yaml.
var ConfigScopes = []string{"user", "repo"}

// ConfigTypes are the value types a config key may hold. Only the scalar
// ones (string, int, bool) are prompted by nerv configure; map and list
// keys are edited in the configuration files and need no prompt.
var ConfigTypes = []string{"string", "int", "bool", "map", "list"}

// ConfigKey is one setting an extension or a provider reads, with what the
// configure wizard shows for it. An extension's keys live under
// <extension>.<key>, a provider's under the path its extension gives its
// providers (such as tasks.providers.<name>.<key>).
type ConfigKey struct {
	Key     string `yaml:"key"`
	Scope   string `yaml:"scope"`
	Type    string `yaml:"type"`
	Default string `yaml:"default"`
	Prompt  string `yaml:"prompt"`
}

// Scalar reports whether the key holds a string, int or bool, the types
// nerv configure prompts for.
func (c ConfigKey) Scalar() bool {
	return c.Type == "string" || c.Type == "int" || c.Type == "bool"
}

// ValidValue reports whether value suits typ: an integer for int, true or
// false for bool; anything else is accepted (string, and the map and list
// types, which take no scalar value).
func ValidValue(typ, value string) bool {
	switch typ {
	case "int":
		_, err := strconv.Atoi(value)
		return err == nil
	case "bool":
		_, err := strconv.ParseBool(value)
		return err == nil
	}
	return true
}

// FindKey returns the key named key of keys.
func FindKey(keys []ConfigKey, key string) (ConfigKey, bool) {
	i := slices.IndexFunc(keys, func(c ConfigKey) bool { return c.Key == key })
	if i < 0 {
		return ConfigKey{}, false
	}
	return keys[i], true
}

// configProblems checks a config block: key names, duplicates, scopes,
// types, scalar defaults and the prompt every scalar key needs.
func configProblems(keys []ConfigKey) []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }
	for i, c := range keys {
		at := fmt.Sprintf("config[%d]", i)
		if !keyRe.MatchString(c.Key) {
			add("%s.key %q must be lower-case letters, digits and underscores", at, c.Key)
		}
		if slices.ContainsFunc(keys[:i], func(o ConfigKey) bool { return o.Key == c.Key }) {
			add("config: %s: duplicated", c.Key)
		}
		if !slices.Contains(ConfigScopes, c.Scope) {
			add("%s.scope %q must be one of %s", at, c.Scope, strings.Join(ConfigScopes, ", "))
		}
		if !slices.Contains(ConfigTypes, c.Type) {
			add("%s.type %q must be one of %s", at, c.Type, strings.Join(ConfigTypes, ", "))
		} else if c.Default != "" && c.Scalar() && !ValidValue(c.Type, c.Default) {
			add("%s.default %q is not a valid %s", at, c.Default, c.Type)
		}
		if strings.TrimSpace(c.Prompt) == "" && c.Scalar() {
			add("%s.prompt is required", at)
		}
	}
	return p
}

// ResolveConfig is the config a provider (or an extension) is called
// with: the values of layers, each overriding the ones before it (such as
// a section-wide value, then a project's), checked against keys, with the
// default of every unset scalar key. A value whose key keys does not
// declare, or that is not of its key's type, is a problem named
// <at>.<key>; problems are reported together, in key order.
//
// The values are YAML-decoded ones: string, int, bool, a map[string]any or
// a []any.
func ResolveConfig(keys []ConfigKey, at string, layers ...map[string]any) (map[string]any, []string) {
	out := map[string]any{}
	var problems []string
	for _, layer := range layers {
		names := make([]string, 0, len(layer))
		for k := range layer {
			names = append(names, k)
		}
		slices.Sort(names)
		for _, name := range names {
			c, ok := FindKey(keys, name)
			if !ok {
				known := make([]string, len(keys))
				for i, k := range keys {
					known[i] = k.Key
				}
				problems = append(problems, fmt.Sprintf("%s.%s: unknown key (want %s)", at, name, orNone(strings.Join(known, ", "))))
				continue
			}
			v := layer[name]
			if v == nil {
				delete(out, name)
				continue
			}
			if !valueIs(v, c.Type) {
				problems = append(problems, fmt.Sprintf("%s.%s: must be a %s", at, name, c.Type))
				continue
			}
			out[name] = v
		}
	}
	for _, c := range keys {
		if _, set := out[c.Key]; set || c.Default == "" || !c.Scalar() {
			continue
		}
		switch c.Type {
		case "int":
			n, _ := strconv.Atoi(c.Default)
			out[c.Key] = n
		case "bool":
			b, _ := strconv.ParseBool(c.Default)
			out[c.Key] = b
		default:
			out[c.Key] = c.Default
		}
	}
	return out, problems
}

// valueIs reports whether a YAML-decoded value holds the config type typ.
func valueIs(v any, typ string) bool {
	switch typ {
	case "string":
		_, ok := v.(string)
		return ok
	case "int":
		_, ok := v.(int)
		return ok
	case "bool":
		_, ok := v.(bool)
		return ok
	case "map":
		_, ok := v.(map[string]any)
		return ok
	case "list":
		_, ok := v.([]any)
		return ok
	}
	return false
}
