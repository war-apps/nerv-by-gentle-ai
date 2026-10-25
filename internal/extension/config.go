package extension

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ConfigScopes are where a config key lives: the user's
// ~/.config/ai-workflow/config.yml or the repository's .ai-workflow.yml.
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
