package deployprovider

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
)

// ConfigScopes are where a provider config key may be set: the user's
// configuration (deploy.providers.<provider>.<key>) or the repository's
// (deploy.targets[].config.<key>, or the user value's project override).
var ConfigScopes = extension.ConfigScopes

// ConfigTypes are the value types a deploy provider config key may hold:
// the scalar types of the common schema.
var ConfigTypes = []string{"string", "int", "bool"}

var variableRe = regexp.MustCompile(`^\{([a-z_]+(?:\.[a-z][a-z0-9_]*)?)\}$`)

// The common contract's types, as a deploy provider uses them.
type (
	ConfigKey = extension.ConfigKey
	Operation = extension.Operation
	Step      = extension.Step
	Arg       = extension.Arg
)

// Manifest is a deploy provider's provider.yaml, as nerv deploy reads it.
type Manifest struct {
	Name            string
	ContractVersion int
	// Slot is SlotHost or SlotTarget.
	Slot   string
	Config []ConfigKey
	// Requires are the executables that must be on PATH.
	Requires []string
	// Operations maps each operation of the slot to its steps (or
	// NotImplemented).
	Operations map[string]Operation
}

// Load reads and validates dir/provider.yaml from fsys (the extensions
// tree): the common contract against the deploy extension's
// extension.yaml, then the deploy step rules.
func Load(fsys fs.FS, dir string) (*Manifest, error) {
	p, ext, err := extension.LoadProvider(fsys, dir)
	if err != nil {
		return nil, err
	}
	if ext.Name != Extension {
		return nil, fmt.Errorf("%s: a provider of extension %s, not a deploy provider", dir, ext.Name)
	}
	m, err := fromProvider(p)
	if err != nil {
		return nil, fmt.Errorf("%s/%w", dir, err)
	}
	return m, nil
}

// Parse decodes and validates a deploy provider.yaml against the deploy
// extension's manifest ext. Unknown fields are errors, and every problem
// found is reported at once.
func Parse(ext *extension.Manifest, data []byte) (*Manifest, error) {
	p, err := extension.ParseProvider(data)
	if err != nil {
		return nil, err
	}
	if err := p.CheckAgainst(ext); err != nil {
		return nil, err
	}
	return fromProvider(p)
}

func fromProvider(p *extension.Provider) (*Manifest, error) {
	m := &Manifest{Name: p.Name, ContractVersion: p.ContractVersion, Slot: p.Slot, Config: p.Config, Requires: p.Requires, Operations: p.Operations}
	var problems []string
	if p.Runtime != extension.RuntimeExec {
		problems = append(problems, fmt.Sprintf("runtime %q: nerv deploy runs exec providers only", p.Runtime))
	}
	problems = append(problems, m.validate()...)
	if len(problems) > 0 {
		return nil, errors.New(ManifestFile + ": " + strings.Join(problems, "; "))
	}
	return m, nil
}

// Implements reports whether the provider implements op.
func (m *Manifest) Implements(op string) bool {
	o, ok := m.Operations[op]
	return ok && !o.NotImplemented
}

// Key returns the config key named key.
func (m *Manifest) Key(key string) (ConfigKey, bool) { return extension.FindKey(m.Config, key) }

// validate checks what only deploy means: scalar config keys, one
// releaseExists step, and each step's for_each, when, absent_stderr and
// argv variables.
func (m *Manifest) validate() []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }
	for i, c := range m.Config {
		if !slices.Contains(ConfigTypes, c.Type) {
			add("config[%d].type %q must be one of %s", i, c.Type, strings.Join(ConfigTypes, ", "))
		}
	}
	for _, op := range Operations[m.Slot] {
		o, ok := m.Operations[op]
		switch {
		case !ok || o.NotImplemented:
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
	if s.ForEach != "" && (op == OpReleaseExists || op == OpPreflight) {
		add("%s.for_each: %s runs once", at, op)
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
	for i, a := range s.Argv {
		where := fmt.Sprintf("%s.argv[%d]", at, i)
		if a.When != "" && !slices.Contains(Booleans, a.When) {
			add("%s.when %q must be one of %s", where, a.When, strings.Join(Booleans, ", "))
		}
		if problem := m.argProblem(a.Value, s.ForEach != ""); problem != "" {
			add("%s: %s", where, problem)
		}
	}
	return p
}

// argProblem checks one element: a literal without braces or exactly one
// known {variable}.
func (m *Manifest) argProblem(v string, item bool) string {
	if !strings.ContainsAny(v, "{}") {
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
func ValidValue(typ, value string) bool { return extension.ValidValue(typ, value) }
