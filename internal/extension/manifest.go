package extension

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest is an extension's extension.yaml: the contract between the core
// and the extension.
type Manifest struct {
	Name            string `yaml:"name"`
	ContractVersion int    `yaml:"contract_version"`
	Description     string `yaml:"description"`
	// Requires are the extensions that must be installed with this one.
	Requires []string `yaml:"requires"`
	// SuggestOrder is where nerv install suggests the extension: lower
	// first, each extension followed by its providers; 0 (unset) comes
	// after every ordered extension, by name.
	SuggestOrder int `yaml:"suggest_order"`
	// Providers is how nerv install installs the extension's providers:
	// ProvidersBundled (every provider shipped is installed with the
	// extension, never asked) or ProvidersSuggested (each is offered after
	// it; the default when unset).
	Providers string `yaml:"providers"`
	// Runtime is how the core reaches the extension: through its entry
	// Agent (RuntimeAgent, as <plugin>:<agent>) or by running Argv
	// (RuntimeExec).
	Runtime string   `yaml:"runtime"`
	Agent   string   `yaml:"agent"`
	Argv    []string `yaml:"argv"`
	// Events are the core events (CoreEvents) the extension subscribes to:
	// the core hands each one to the extension's entry.
	Events []string `yaml:"events"`
	// Config are the extension's own keys, under <extension>.<key>.
	Config []ConfigKey `yaml:"config"`
	// ProviderFields are the extension-specific top-level fields its
	// providers' provider.yaml may (or, when required, must) carry.
	ProviderFields []Field `yaml:"provider_fields"`
	// Slots are the kinds of provider the extension takes.
	Slots []Slot `yaml:"slots"`
}

// Field is an extension-specific provider.yaml field: its name, its YAML
// type (string, int, bool, map or list) and whether every provider must
// set it. The owning extension's own package checks its meaning.
type Field struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
}

// Slot is one kind of provider an extension takes, such as deploy's host
// and target.
type Slot struct {
	Name        string `yaml:"name"`
	Cardinality string `yaml:"cardinality"`
	// Default is the provider used when the configuration names none.
	Default string `yaml:"default"`
	// Operations maps each operation a provider of the slot implements to
	// Required or Optional (only an optional one may be NotImplemented).
	Operations map[string]string `yaml:"operations"`
	// Events maps each core event the extension hands the slot's providers
	// to the slot operations handling it needs: a provider that handles
	// the event (its provider.yaml events) cannot declare one of them
	// NotImplemented.
	Events map[string][]string `yaml:"events"`
}

// OperationNames returns the slot's operations in name order.
func (s Slot) OperationNames() []string { return slices.Sorted(maps.Keys(s.Operations)) }

// EventNames returns the events the slot hands its providers, in name
// order.
func (s Slot) EventNames() []string { return slices.Sorted(maps.Keys(s.Events)) }

// Bundles reports whether the extension's providers are installed with
// it (providers: bundled).
func (m *Manifest) Bundles() bool { return m.Providers == ProvidersBundled }

// Slot returns the slot named name.
func (m *Manifest) Slot(name string) (Slot, bool) {
	i := slices.IndexFunc(m.Slots, func(s Slot) bool { return s.Name == name })
	if i < 0 {
		return Slot{}, false
	}
	return m.Slots[i], true
}

// Subscribes reports whether the extension subscribes to the core event.
func (m *Manifest) Subscribes(event string) bool {
	return slices.Contains(m.Events, event)
}

// ParseManifest decodes and validates an extension.yaml. Unknown fields are
// errors, and every problem found is reported at once.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := decodeStrict(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	if problems := m.problems(); len(problems) > 0 {
		return nil, errors.New(ManifestFile + ": " + strings.Join(problems, "; "))
	}
	return &m, nil
}

// LoadManifest reads and validates dir/extension.yaml from fsys; the
// extension's name must be its folder's.
func LoadManifest(fsys fs.FS, dir string) (*Manifest, error) {
	data, err := fs.ReadFile(fsys, dir+"/"+ManifestFile)
	if err != nil {
		return nil, err
	}
	m, err := ParseManifest(data)
	if err != nil {
		// ParseManifest's errors start with "extension.yaml: ": this makes
		// them "<dir>/extension.yaml: ...", the manifest's path in fsys.
		return nil, fmt.Errorf("%s/%w", dir, err)
	}
	if m.Name != dir {
		return nil, fmt.Errorf("%s/%s: name %q must match its folder %q", dir, ManifestFile, m.Name, dir)
	}
	return m, nil
}

func (m *Manifest) problems() []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }

	if !ValidName(m.Name) {
		add("name %q must be lower-case letters, digits and dashes", m.Name)
	}
	if m.ContractVersion != ContractVersion {
		add("contract_version %d is not supported (this nerv understands %d)", m.ContractVersion, ContractVersion)
	}
	if m.Providers != "" && !slices.Contains(ProvidersModes, m.Providers) {
		add("providers %q must be one of %s", m.Providers, strings.Join(ProvidersModes, ", "))
	}
	if m.SuggestOrder < 0 {
		add("suggest_order %d must be 0 (unset) or positive", m.SuggestOrder)
	}
	for i, req := range m.Requires {
		switch {
		case !ValidName(req):
			add("requires[%d] %q must be an extension name", i, req)
		case req == m.Name:
			add("requires: %s: an extension cannot require itself", req)
		case slices.Contains(m.Requires[:i], req):
			add("requires: %s: duplicated", req)
		}
	}

	switch m.Runtime {
	case RuntimeAgent:
		if !agentRe.MatchString(m.Agent) {
			add("agent %q must name the entry agent as <plugin>:<agent>", m.Agent)
		}
		if len(m.Argv) > 0 {
			add("argv is for runtime exec, not agent")
		}
	case RuntimeExec:
		p = append(p, argvProblems("argv", m.Argv)...)
		if m.Agent != "" {
			add("agent is for runtime agent, not exec")
		}
	default:
		add("runtime %q must be one of %s", m.Runtime, strings.Join(Runtimes, ", "))
	}

	for i, event := range m.Events {
		if !slices.Contains(CoreEvents, event) {
			add("events: %s is not a core event (%s)", event, strings.Join(CoreEvents, ", "))
		}
		if slices.Contains(m.Events[:i], event) {
			add("events: %s: duplicated", event)
		}
	}
	p = append(p, configProblems(m.Config)...)

	for i, f := range m.ProviderFields {
		at := fmt.Sprintf("provider_fields[%d]", i)
		if !keyRe.MatchString(f.Name) {
			add("%s.name %q must be lower-case letters, digits and underscores", at, f.Name)
		}
		if commonProviderFields[f.Name] {
			add("%s.name %q is a common provider.yaml field", at, f.Name)
		}
		if slices.ContainsFunc(m.ProviderFields[:i], func(o Field) bool { return o.Name == f.Name }) {
			add("provider_fields: %s: duplicated", f.Name)
		}
		if !slices.Contains(ConfigTypes, f.Type) {
			add("%s.type %q must be one of %s", at, f.Type, strings.Join(ConfigTypes, ", "))
		}
	}

	for i, s := range m.Slots {
		at := fmt.Sprintf("slots[%d]", i)
		if !ValidName(s.Name) {
			add("%s.name %q must be lower-case letters, digits and dashes", at, s.Name)
		}
		if slices.ContainsFunc(m.Slots[:i], func(o Slot) bool { return o.Name == s.Name }) {
			add("slots: %s: duplicated", s.Name)
		}
		if !slices.Contains(Cardinalities, s.Cardinality) {
			add("%s.cardinality %q must be one of %s", at, s.Cardinality, strings.Join(Cardinalities, ", "))
		}
		if s.Default != "" && !ValidName(s.Default) {
			add("%s.default %q must be a provider name", at, s.Default)
		}
		if len(s.Operations) == 0 {
			add("%s.operations: a slot needs at least one operation", at)
		}
		for _, op := range s.OperationNames() {
			if !opRe.MatchString(op) {
				add("%s.operations.%s: an operation name is lowerCamelCase", at, op)
			}
			if level := s.Operations[op]; level != Required && level != Optional {
				add("%s.operations.%s %q must be %s or %s", at, op, level, Required, Optional)
			}
		}
		for _, event := range s.EventNames() {
			ops := s.Events[event]
			if !m.Subscribes(event) {
				add("%s.events.%s: the extension does not subscribe to it (events)", at, event)
			}
			if len(ops) == 0 {
				add("%s.events.%s: an event needs at least one operation", at, event)
			}
			for _, op := range ops {
				if _, ok := s.Operations[op]; !ok {
					add("%s.events.%s: %s is not an operation of the slot", at, event, op)
				}
			}
		}
	}
	return p
}

// RequiresProblems checks the requires of a tree's extensions together,
// which one manifest alone cannot: requires maps each extension to its
// requires, and known reports whether a name is an extension of the tree
// (an extension outside requires, such as a shipped one a project
// extension requires, has requires of its own already checked). It
// returns every requirement known refuses, then every requires cycle once,
// each named by the manifest that starts it: "<name>/extension.yaml: ...".
func RequiresProblems(requires map[string][]string, known func(string) bool) []string {
	names := slices.Sorted(maps.Keys(requires))
	var p []string
	for _, name := range names {
		for _, req := range requires[name] {
			if !known(req) {
				p = append(p, fmt.Sprintf("%s/%s: requires: %s is not an extension of this tree", name, ManifestFile, req))
			}
		}
	}

	const (
		unvisited = iota
		onPath
		done
	)
	state := map[string]int{}
	var path []string
	var visit func(name string)
	visit = func(name string) {
		state[name] = onPath
		path = append(path, name)
		for _, req := range requires[name] {
			if _, inTree := requires[req]; !inTree {
				continue
			}
			switch state[req] {
			case onPath:
				cycle := append(slices.Clone(path[slices.Index(path, req):]), req)
				p = append(p, fmt.Sprintf("%s/%s: requires cycle: %s", req, ManifestFile, strings.Join(cycle, " -> ")))
			case unvisited:
				visit(req)
			}
		}
		path = path[:len(path)-1]
		state[name] = done
	}
	for _, name := range names {
		if state[name] == unvisited {
			visit(name)
		}
	}
	return p
}

// argvProblems checks an exec argv: a command ({nerv} or a bare executable
// name) and non-empty arguments.
func argvProblems(at string, argv []string) []string {
	if len(argv) == 0 {
		return []string{at + " is required for runtime exec"}
	}
	var p []string
	if cmd := argv[0]; cmd != "{nerv}" && !toolRe.MatchString(cmd) {
		p = append(p, fmt.Sprintf("%s[0] %q must be {nerv} or an executable name (no path)", at, cmd))
	}
	for i, a := range argv[1:] {
		if a == "" {
			p = append(p, fmt.Sprintf("%s[%d]: an empty argument", at, i+1))
		}
	}
	return p
}

// decodeStrict decodes one YAML document into v, refusing fields v does
// not declare.
func decodeStrict(data []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(v)
}
