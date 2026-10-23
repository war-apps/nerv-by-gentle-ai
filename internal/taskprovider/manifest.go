package taskprovider

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFile is the manifest's name at the root of a provider plugin.
const ManifestFile = "provider.yaml"

// ConfigScopes are where a provider config key lives: the user's
// ~/.claude/nerv/nerv.yaml or the repository's .nerv/nerv.yaml.
var ConfigScopes = []string{"user", "repo"}

// ConfigTypes are the value types a provider config key may hold. Only the
// scalar ones (string, int, bool) are prompted by nerv configure; map and
// list keys are edited in nerv.yaml and need no prompt.
var ConfigTypes = []string{"string", "int", "bool", "map", "list"}

var (
	nameRe   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	prefixRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)
	keyRe    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// Manifest is a provider's provider.yaml. Its config keys are read from
// tasks.providers.<name>.<key> in nerv.yaml.
type Manifest struct {
	Name            string            `yaml:"name"`
	ContractVersion int               `yaml:"contract_version"`
	TaskRefPrefix   string            `yaml:"task_ref_prefix"`
	MCP             MCP               `yaml:"mcp"`
	Config          []ConfigKey       `yaml:"config"`
	Stages          map[string]string `yaml:"stages"`
	Operations      map[string]string `yaml:"operations"`
	Events          []string          `yaml:"events"`
}

// MCP names the server and the exact tools the provider calls, so the
// dispatcher can tell a missing connector from a provider failure.
type MCP struct {
	Server string   `yaml:"server"`
	Tools  []string `yaml:"tools"`
}

// ConfigKey is one setting the provider needs, with what the configure
// wizard shows for it.
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

// Parse decodes and validates a provider.yaml. Unknown fields are errors,
// and every problem found is reported at once.
func Parse(data []byte) (*Manifest, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("provider.yaml: %w", err)
	}
	if problems := m.validate(); len(problems) > 0 {
		return nil, errors.New("provider.yaml: " + strings.Join(problems, "; "))
	}
	return &m, nil
}

// Implements reports whether the provider implements op.
func (m *Manifest) Implements(op string) bool {
	file, ok := m.Operations[op]
	return ok && file != NotImplemented
}

// Subscribes reports whether the provider handles event.
func (m *Manifest) Subscribes(event string) bool {
	return slices.Contains(m.Events, event)
}

// CheckFiles reports every operation file the manifest names that fsys
// (the provider plugin's root) does not hold.
func (m *Manifest) CheckFiles(fsys fs.FS) error {
	var missing []string
	for _, op := range Operations {
		if !m.Implements(op) {
			continue
		}
		file := m.Operations[op]
		if _, err := fs.Stat(fsys, file); err != nil {
			missing = append(missing, fmt.Sprintf("operations.%s: %s not found", op, file))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("provider %s: %s", m.Name, strings.Join(missing, "; "))
	}
	return nil
}

// MissingTools returns the declared tools absent from available, in
// declaration order. A tool counts when available holds its bare name or
// its Claude Code name mcp__<server>__<tool>; the same tool name served by
// another MCP server does not.
func (m *Manifest) MissingTools(available []string) []string {
	var missing []string
	for _, tool := range m.MCP.Tools {
		qualified := "mcp__" + m.MCP.Server + "__" + tool
		if !slices.Contains(available, tool) && !slices.Contains(available, qualified) {
			missing = append(missing, tool)
		}
	}
	return missing
}

func (m *Manifest) validate() []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }

	if !nameRe.MatchString(m.Name) {
		add("name %q must be lower-case letters, digits and dashes", m.Name)
	}
	if m.ContractVersion != ContractVersion {
		add("contract_version %d is not supported (this nerv understands %d)", m.ContractVersion, ContractVersion)
	}
	if !prefixRe.MatchString(m.TaskRefPrefix) {
		add("task_ref_prefix %q must be upper-case letters and digits", m.TaskRefPrefix)
	}

	if m.MCP.Server == "" {
		add("mcp.server is required")
	}
	if len(m.MCP.Tools) == 0 {
		add("mcp.tools must name every tool the provider calls")
	}
	for i, tool := range m.MCP.Tools {
		if slices.Contains(m.MCP.Tools[:i], tool) {
			add("mcp.tools: %s: duplicated", tool)
		}
	}

	for i, c := range m.Config {
		at := fmt.Sprintf("config[%d]", i)
		if !keyRe.MatchString(c.Key) {
			add("%s.key %q must be lower-case letters, digits and underscores", at, c.Key)
		}
		if slices.ContainsFunc(m.Config[:i], func(o ConfigKey) bool { return o.Key == c.Key }) {
			add("config: %s: duplicated", c.Key)
		}
		if !slices.Contains(ConfigScopes, c.Scope) {
			add("%s.scope %q must be one of %s", at, c.Scope, strings.Join(ConfigScopes, ", "))
		}
		if !slices.Contains(ConfigTypes, c.Type) {
			add("%s.type %q must be one of %s", at, c.Type, strings.Join(ConfigTypes, ", "))
		} else if c.Default != "" && c.Scalar() && !validValue(c.Type, c.Default) {
			add("%s.default %q is not a valid %s", at, c.Default, c.Type)
		}
		if strings.TrimSpace(c.Prompt) == "" && c.Scalar() {
			add("%s.prompt is required", at)
		}
	}

	for _, key := range sortedKeys(m.Stages) {
		if !slices.Contains(StageKeys, key) {
			add("stages.%s is not a canonical stage (%s)", key, strings.Join(StageKeys, ", "))
		}
	}

	for _, op := range sortedKeys(m.Operations) {
		if !slices.Contains(Operations, op) {
			add("operations.%s is not a contract operation", op)
		}
	}
	for _, op := range Operations {
		file, ok := m.Operations[op]
		switch {
		case !ok:
			add("operations.%s: missing (implement it or declare %s)", op, NotImplemented)
		case file == NotImplemented:
		case path.IsAbs(file) || !fs.ValidPath(file) || path.Ext(file) != ".md":
			add("operations.%s: %q must be a relative .md path inside the plugin", op, file)
		}
	}

	for i, event := range m.Events {
		needs, ok := Events[event]
		if !ok {
			add("events: %s is not a contract event", event)
			continue
		}
		if slices.Contains(m.Events[:i], event) {
			add("events: %s: duplicated", event)
		}
		for _, op := range needs {
			if _, declared := m.Operations[op]; declared && !m.Implements(op) {
				add("events: %s needs %s, declared %s", event, op, NotImplemented)
			}
		}
	}
	return p
}

func validValue(typ, value string) bool {
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

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
