package deployprovider

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigScopes are where a provider config key may be set: the user's
// configuration (deploy.providers.<provider>.<key>) or the repository's
// (deploy.targets[].config.<key>, or the user value's project override).
var ConfigScopes = []string{"user", "repo"}

// ConfigTypes are the value types a config key may hold.
var ConfigTypes = []string{"string", "int", "bool"}

var (
	nameRe     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	keyRe      = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	toolRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
	variableRe = regexp.MustCompile(`^\{([a-z_]+(?:\.[a-z][a-z0-9_]*)?)\}$`)
)

// Manifest is a deploy provider's deploy-provider.yaml.
type Manifest struct {
	Name            string      `yaml:"name"`
	ContractVersion int         `yaml:"contract_version"`
	Kind            string      `yaml:"kind"`
	Config          []ConfigKey `yaml:"config"`
	// Requires are the executables that must be on PATH.
	Requires []string `yaml:"requires"`
	// Operations maps each operation of the kind to its steps (nil for
	// NotImplemented).
	Operations map[string]Operation `yaml:"operations"`
}

// ConfigKey is one setting the provider reads.
type ConfigKey struct {
	Key     string `yaml:"key"`
	Scope   string `yaml:"scope"`
	Type    string `yaml:"type"`
	Default string `yaml:"default"`
	Prompt  string `yaml:"prompt"`
}

// Operation is the steps of one operation, run in order; nil steps with
// NotImplemented set means the provider does not support it.
type Operation struct {
	NotImplemented bool
	Steps          []Step
}

// UnmarshalYAML accepts the scalar not_implemented or a list of steps.
func (o *Operation) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		if node.Value != NotImplemented {
			return fmt.Errorf("line %d: an operation is a list of steps or %s, not %q", node.Line, NotImplemented, node.Value)
		}
		o.NotImplemented = true
		return nil
	}
	return node.Decode(&o.Steps)
}

// Step is one command of an operation.
type Step struct {
	// Argv is the command and its arguments, run without a shell.
	Argv []Arg `yaml:"argv"`
	// ForEach runs the step once per item of one of ForEach, with {item}.
	ForEach string `yaml:"for_each"`
	// When runs the step only when the boolean variable is true.
	When string `yaml:"when"`
	// AbsentStderr, for releaseExists only, is a regular expression: a
	// non-zero exit whose stderr matches it means the release does not
	// exist (any other non-zero exit is a failure).
	AbsentStderr string `yaml:"absent_stderr"`
}

// Arg is one argv element: a literal, a whole {variable}, or either one
// kept only when a boolean variable is true ({arg: --prerelease, when:
// prerelease}).
type Arg struct {
	Value string `yaml:"arg"`
	When  string `yaml:"when"`
}

// UnmarshalYAML accepts a scalar or the {arg, when} mapping.
func (a *Arg) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		a.Value = node.Value
		return nil
	}
	type plain Arg
	var p plain
	if err := decodeKnown(node, &p); err != nil {
		return err
	}
	*a = Arg(p)
	return nil
}

// decodeKnown decodes node into v, refusing keys v does not declare.
func decodeKnown(node *yaml.Node, v any) error {
	data, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(v)
}

// Parse decodes and validates a deploy-provider.yaml. Unknown fields are
// errors, and every problem found is reported at once.
func Parse(data []byte) (*Manifest, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	if problems := m.validate(); len(problems) > 0 {
		return nil, errors.New(ManifestFile + ": " + strings.Join(problems, "; "))
	}
	return &m, nil
}

// Implements reports whether the provider implements op.
func (m *Manifest) Implements(op string) bool {
	o, ok := m.Operations[op]
	return ok && !o.NotImplemented
}

// Key returns the config key named key.
func (m *Manifest) Key(key string) (ConfigKey, bool) {
	i := slices.IndexFunc(m.Config, func(c ConfigKey) bool { return c.Key == key })
	if i < 0 {
		return ConfigKey{}, false
	}
	return m.Config[i], true
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
	ops, kindOK := Operations[m.Kind]
	if !kindOK {
		add("kind %q must be one of %s", m.Kind, strings.Join(Kinds, ", "))
	}

	for i, tool := range m.Requires {
		if !toolRe.MatchString(tool) {
			add("requires[%d] %q must be an executable name (no path)", i, tool)
		}
		if slices.Contains(m.Requires[:i], tool) {
			add("requires: %s: duplicated", tool)
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
		} else if c.Default != "" && !ValidValue(c.Type, c.Default) {
			add("%s.default %q is not a valid %s", at, c.Default, c.Type)
		}
		if strings.TrimSpace(c.Prompt) == "" {
			add("%s.prompt is required", at)
		}
	}

	if !kindOK {
		return p
	}
	for _, op := range sortedOps(m.Operations) {
		if !slices.Contains(ops, op) {
			add("operations.%s is not an operation of kind %s (%s)", op, m.Kind, strings.Join(ops, ", "))
		}
	}
	for _, op := range ops {
		o, ok := m.Operations[op]
		switch {
		case !ok:
			add("operations.%s: missing", op)
		case o.NotImplemented && !slices.Contains(Optional, op):
			add("operations.%s: required, it cannot be %s", op, NotImplemented)
		case o.NotImplemented:
		case len(o.Steps) == 0:
			add("operations.%s: no steps", op)
		case op == OpReleaseExists && len(o.Steps) != 1:
			add("operations.%s: exactly one step, whose exit code and output answer it", op)
		default:
			for i, s := range o.Steps {
				p = append(p, m.validateStep(fmt.Sprintf("operations.%s[%d]", op, i), op, s)...)
			}
		}
	}
	return p
}

func (m *Manifest) validateStep(at, op string, s Step) []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }
	if s.ForEach != "" && !slices.Contains(ForEach, s.ForEach) {
		add("%s.for_each %q must be one of %s", at, s.ForEach, strings.Join(ForEach, ", "))
	}
	if s.ForEach != "" && op == OpReleaseExists {
		add("%s.for_each: releaseExists runs once", at)
	}
	if s.When != "" && !slices.Contains(Booleans, s.When) {
		add("%s.when %q must be one of %s", at, s.When, strings.Join(Booleans, ", "))
	}
	if s.AbsentStderr != "" {
		if op != OpReleaseExists {
			add("%s.absent_stderr is only for releaseExists", at)
		} else if _, err := regexp.Compile(s.AbsentStderr); err != nil {
			add("%s.absent_stderr: %v", at, err)
		}
	}
	if len(s.Argv) == 0 {
		add("%s.argv is required", at)
		return p
	}
	for i, a := range s.Argv {
		where := fmt.Sprintf("%s.argv[%d]", at, i)
		if a.When != "" && !slices.Contains(Booleans, a.When) {
			add("%s.when %q must be one of %s", where, a.When, strings.Join(Booleans, ", "))
		}
		if i == 0 && a.When != "" {
			add("%s: the command cannot be conditional", where)
		}
		if problem := m.argProblem(a.Value, s.ForEach != ""); problem != "" {
			add("%s: %s", where, problem)
		}
	}
	if cmd := s.Argv[0].Value; cmd != "{nerv}" && !slices.Contains(m.Requires, cmd) {
		add("%s.argv[0] %q must be {nerv} or an executable listed in requires", at, cmd)
	}
	return p
}

// argProblem checks one element: a literal without braces or exactly one
// known {variable}.
func (m *Manifest) argProblem(v string, item bool) string {
	if !strings.ContainsAny(v, "{}") {
		if v == "" {
			return "an empty argument"
		}
		return ""
	}
	match := variableRe.FindStringSubmatch(v)
	if match == nil {
		return fmt.Sprintf("%q: a variable must be the whole argument, as {name}", v)
	}
	name := match[1]
	switch key, isConfig := strings.CutPrefix(name, "config."); {
	case isConfig:
		if _, ok := m.Key(key); !ok {
			return fmt.Sprintf("%q: config key %s is not declared", v, key)
		}
	case name == "item":
		if !item {
			return fmt.Sprintf("%q: {item} needs for_each", v)
		}
	case !slices.Contains(Variables, name):
		return fmt.Sprintf("%q: unknown variable (known: %s, config.<key>, item)", v, strings.Join(Variables, ", "))
	}
	return ""
}

// ValidValue reports whether value suits typ.
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

func sortedOps(m map[string]Operation) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
