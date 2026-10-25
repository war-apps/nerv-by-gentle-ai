package extension

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Provider is a provider's provider.yaml: the contract between an
// extension and one of its providers. Every provider, of every extension,
// has this shape; the owning extension's extension.yaml says which slots
// exist, which operations each needs and which extra top-level fields
// (Fields) its providers carry.
type Provider struct {
	Name            string `yaml:"name"`
	Extension       string `yaml:"extension"`
	Slot            string `yaml:"slot"`
	ContractVersion int    `yaml:"contract_version"`
	Runtime         string `yaml:"runtime"`
	// Requires are the executables an exec provider needs on PATH.
	Requires []string `yaml:"requires"`
	// MCP names the server and tools an agent provider calls.
	MCP    MCP         `yaml:"mcp"`
	Config []ConfigKey `yaml:"config"`
	// Operations maps every slot operation to its implementation.
	Operations map[string]Operation `yaml:"operations"`
	// Events are the events of its slot's events the provider handles;
	// each needs the operations the slot lists for it.
	Events []string `yaml:"events"`
	// Fields are the extension-specific top-level fields, by name, as the
	// owning extension's provider_fields declare them; DecodeField reads
	// one.
	Fields map[string]*yaml.Node `yaml:"-"`
}

// commonProviderFields are the provider.yaml fields every provider shares;
// any other top-level field must be one of the owning extension's
// provider_fields.
var commonProviderFields = map[string]bool{
	"name": true, "extension": true, "slot": true, "contract_version": true, "runtime": true,
	"requires": true, "mcp": true, "config": true, "operations": true, "events": true,
}

// MCP names the server and the exact tools an agent provider calls, so its
// extension can tell a missing connector from a provider failure.
type MCP struct {
	Server string   `yaml:"server"`
	Tools  []string `yaml:"tools"`
}

// Operation is one slot operation of a provider: NotImplemented, a
// procedure file an agent follows (runtime agent), or the steps Go runs
// (runtime exec).
type Operation struct {
	NotImplemented bool
	Procedure      string
	Steps          []Step
}

// UnmarshalYAML accepts the scalar not_implemented, a procedure path or a
// list of steps.
func (o *Operation) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		if node.Value == NotImplemented {
			o.NotImplemented = true
		} else {
			o.Procedure = node.Value
		}
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: an operation is a procedure file, a list of steps or %s", node.Line, NotImplemented)
	}
	return node.Decode(&o.Steps)
}

// Step is one command of an exec operation, run without a shell.
type Step struct {
	// Argv is the command and its arguments; an element is a literal or
	// exactly one {variable} the owning extension defines.
	Argv []Arg `yaml:"argv"`
	// ForEach runs the step once per item of a list the extension defines,
	// with {item}.
	ForEach string `yaml:"for_each"`
	// When runs the step only when the boolean variable is true.
	When string `yaml:"when"`
	// AbsentStderr is a regular expression an extension may give meaning
	// to (deploy: a releaseExists exit whose stderr matches means absent).
	AbsentStderr string `yaml:"absent_stderr"`
}

// UnmarshalYAML refuses step fields Step does not declare.
func (s *Step) UnmarshalYAML(node *yaml.Node) error {
	type plain Step
	var p plain
	if err := decodeKnown(node, &p); err != nil {
		return err
	}
	*s = Step(p)
	return nil
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
	return decodeStrict(data, v)
}

// Implements reports whether the provider implements op.
func (p *Provider) Implements(op string) bool {
	o, ok := p.Operations[op]
	return ok && !o.NotImplemented
}

// Key returns the config key named key.
func (p *Provider) Key(key string) (ConfigKey, bool) { return FindKey(p.Config, key) }

// DecodeField decodes the extension field name into v; an absent field
// leaves v untouched.
func (p *Provider) DecodeField(name string, v any) error {
	node, ok := p.Fields[name]
	if !ok {
		return nil
	}
	if err := node.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// ParseProvider decodes a provider.yaml and checks what does not depend on
// its extension. Unknown common fields are errors; any other top-level
// field is kept in Fields for CheckAgainst to accept or refuse.
func ParseProvider(data []byte) (*Provider, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", ProviderFile, err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: the manifest must be a mapping", ProviderFile)
	}
	root := doc.Content[0]
	common := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	fields := map[string]*yaml.Node{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if commonProviderFields[key.Value] {
			common.Content = append(common.Content, key, value)
			continue
		}
		if _, dup := fields[key.Value]; dup {
			return nil, fmt.Errorf("%s: line %d: field %s duplicated", ProviderFile, key.Line, key.Value)
		}
		fields[key.Value] = value
	}
	var p Provider
	if err := decodeKnown(common, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", ProviderFile, err)
	}
	p.Fields = fields
	if problems := p.problems(); len(problems) > 0 {
		return nil, errors.New(ProviderFile + ": " + strings.Join(problems, "; "))
	}
	return &p, nil
}

func (p *Provider) problems() []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }

	if !ValidName(p.Name) {
		add("name %q must be lower-case letters, digits and dashes", p.Name)
	}
	if !ValidName(p.Extension) {
		add("extension %q must name the extension the provider belongs to", p.Extension)
	}
	if !ValidName(p.Slot) {
		add("slot %q must name a slot of the extension", p.Slot)
	}
	if p.ContractVersion != ContractVersion {
		add("contract_version %d is not supported (this nerv understands %d)", p.ContractVersion, ContractVersion)
	}

	switch p.Runtime {
	case RuntimeAgent:
		if len(p.Requires) > 0 {
			add("requires is for runtime exec; an agent provider names its mcp tools")
		}
		if p.MCP.Server == "" {
			add("mcp.server is required")
		}
		if len(p.MCP.Tools) == 0 {
			add("mcp.tools must name every tool the provider calls")
		}
		for i, tool := range p.MCP.Tools {
			if slices.Contains(p.MCP.Tools[:i], tool) {
				add("mcp.tools: %s: duplicated", tool)
			}
		}
	case RuntimeExec:
		if p.MCP.Server != "" || len(p.MCP.Tools) > 0 {
			add("mcp is for runtime agent, not exec")
		}
		for i, tool := range p.Requires {
			if !toolRe.MatchString(tool) {
				add("requires[%d] %q must be an executable name (no path)", i, tool)
			}
			if slices.Contains(p.Requires[:i], tool) {
				add("requires: %s: duplicated", tool)
			}
		}
	default:
		add("runtime %q must be one of %s", p.Runtime, strings.Join(Runtimes, ", "))
	}

	out = append(out, configProblems(p.Config)...)

	for i, event := range p.Events {
		if slices.Contains(p.Events[:i], event) {
			add("events: %s: duplicated", event)
		}
	}

	for _, op := range p.operationNames() {
		o := p.Operations[op]
		at := "operations." + op
		switch {
		case o.NotImplemented:
		case p.Runtime == RuntimeAgent && o.Procedure == "":
			add("%s: an agent provider names a procedure .md file", at)
		case p.Runtime == RuntimeAgent && (path.IsAbs(o.Procedure) || !fs.ValidPath(o.Procedure) || path.Ext(o.Procedure) != ".md"):
			add("%s: %q must be a relative .md path inside the provider", at, o.Procedure)
		case p.Runtime == RuntimeExec && o.Procedure != "":
			add("%s: an operation is a list of steps or %s, not %q", at, NotImplemented, o.Procedure)
		case p.Runtime == RuntimeExec && len(o.Steps) == 0:
			add("%s: no steps", at)
		case p.Runtime == RuntimeExec:
			for i, s := range o.Steps {
				out = append(out, p.stepProblems(fmt.Sprintf("%s[%d]", at, i), s)...)
			}
		}
	}
	return out
}

// stepProblems checks what every exec step needs: an argv whose command
// is {nerv} or a required executable, never conditional, and no empty
// argument. The variables, for_each lists and when conditions belong to
// the owning extension.
func (p *Provider) stepProblems(at string, s Step) []string {
	if len(s.Argv) == 0 {
		return []string{at + ".argv is required"}
	}
	var out []string
	for i, a := range s.Argv {
		if a.Value == "" {
			out = append(out, fmt.Sprintf("%s.argv[%d]: an empty argument", at, i))
		}
	}
	if s.Argv[0].When != "" {
		out = append(out, fmt.Sprintf("%s.argv[0]: the command cannot be conditional", at))
	}
	if cmd := s.Argv[0].Value; cmd != "{nerv}" && !slices.Contains(p.Requires, cmd) {
		out = append(out, fmt.Sprintf("%s.argv[0] %q must be {nerv} or an executable listed in requires", at, cmd))
	}
	return out
}

// CheckAgainst checks the provider against its owning extension's
// manifest: the extension and slot match, every slot operation is declared
// and no other, a required one is not NotImplemented, and every extra
// field is one of the extension's provider_fields, of its type, with the
// required ones present.
func (p *Provider) CheckAgainst(ext *Manifest) error {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	if p.Extension != ext.Name {
		add("extension %q is not %s", p.Extension, ext.Name)
	}
	slot, ok := ext.Slot(p.Slot)
	if !ok {
		var names []string
		for _, s := range ext.Slots {
			names = append(names, s.Name)
		}
		add("slot %q is not a slot of extension %s (%s)", p.Slot, ext.Name, orNone(strings.Join(names, ", ")))
	} else {
		ops := slot.OperationNames()
		for _, op := range p.operationNames() {
			if !slices.Contains(ops, op) {
				add("operations.%s is not an operation of slot %s.%s (%s)", op, ext.Name, slot.Name, strings.Join(ops, ", "))
			}
		}
		for _, op := range ops {
			o, declared := p.Operations[op]
			switch {
			case !declared:
				add("operations.%s: missing (implement it or declare %s)", op, NotImplemented)
			case o.NotImplemented && slot.Operations[op] == Required:
				add("operations.%s: required by slot %s.%s, it cannot be %s", op, ext.Name, slot.Name, NotImplemented)
			}
		}
		out = append(out, p.eventProblems(ext.Name, slot)...)
	}

	for _, name := range sortedNames(p.Fields) {
		i := slices.IndexFunc(ext.ProviderFields, func(f Field) bool { return f.Name == name })
		if i < 0 {
			add("field %s not found: not a common provider.yaml field nor one of extension %s's provider_fields", name, ext.Name)
			continue
		}
		if f := ext.ProviderFields[i]; !nodeIs(p.Fields[name], f.Type) {
			add("%s must be a %s", name, f.Type)
		}
	}
	for _, f := range ext.ProviderFields {
		if _, set := p.Fields[f.Name]; f.Required && !set {
			add("%s is required by extension %s", f.Name, ext.Name)
		}
	}
	if len(out) > 0 {
		return errors.New(ProviderFile + ": " + strings.Join(out, "; "))
	}
	return nil
}

// eventProblems checks the provider's events against its slot: each is
// one the slot hands its providers, and none needs an operation the
// provider declares NotImplemented.
func (p *Provider) eventProblems(ext string, slot Slot) []string {
	if len(p.Events) == 0 {
		return nil
	}
	if len(slot.Events) == 0 {
		return []string{fmt.Sprintf("events: slot %s.%s hands its providers no events", ext, slot.Name)}
	}
	var out []string
	for _, event := range p.Events {
		needs, ok := slot.Events[event]
		if !ok {
			out = append(out, fmt.Sprintf("events: %s is not an event slot %s.%s hands its providers (%s)", event, ext, slot.Name, strings.Join(slot.EventNames(), ", ")))
			continue
		}
		for _, op := range needs {
			if o, declared := p.Operations[op]; declared && o.NotImplemented {
				out = append(out, fmt.Sprintf("events: %s needs %s, declared %s", event, op, NotImplemented))
			}
		}
	}
	return out
}

// CheckFiles reports every procedure file the provider names that fsys
// (the provider's root) does not hold.
func (p *Provider) CheckFiles(fsys fs.FS) error {
	var missing []string
	for _, op := range p.operationNames() {
		file := p.Operations[op].Procedure
		if file == "" || p.Operations[op].NotImplemented {
			continue
		}
		if _, err := fs.Stat(fsys, file); err != nil {
			missing = append(missing, fmt.Sprintf("operations.%s: %s not found", op, file))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("provider %s: %s", p.Name, strings.Join(missing, "; "))
	}
	return nil
}

// LoadProvider reads dir/provider.yaml from fsys (a tree with one folder
// per extension and provider) and checks it whole: the folder is
// <extension>-<name>, the owning extension's folder holds a valid
// extension.yaml, the provider fits its slot and its procedure files
// exist. A project provider's {provider_dir} paths (see WithProject) are
// resolved to absolute paths inside its folder. It returns the provider
// and its extension's manifest.
func LoadProvider(fsys fs.FS, dir string) (*Provider, *Manifest, error) {
	data, err := fs.ReadFile(fsys, dir+"/"+ProviderFile)
	if err != nil {
		return nil, nil, err
	}
	p, err := ParseProvider(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s/%w", dir, err)
	}
	if want := p.Extension + "-" + p.Name; dir != want {
		return nil, nil, fmt.Errorf("%s/%s: a provider of extension %s named %s lives in the folder %s", dir, ProviderFile, p.Extension, p.Name, want)
	}
	if _, err := fs.Stat(fsys, p.Extension+"/"+ManifestFile); err != nil {
		return nil, nil, fmt.Errorf("%s/%s: extension %q is not available (no %s/%s)", dir, ProviderFile, p.Extension, p.Extension, ManifestFile)
	}
	ext, err := LoadManifest(fsys, p.Extension)
	if err != nil {
		return nil, nil, err
	}
	if err := p.CheckAgainst(ext); err != nil {
		return nil, nil, fmt.Errorf("%s/%w", dir, err)
	}
	root, err := fs.Sub(fsys, dir)
	if err != nil {
		return nil, nil, err
	}
	if err := p.CheckFiles(root); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", dir, err)
	}
	var folder *projectFolder
	if t, ok := fsys.(*projectTree); ok {
		if f, ok := t.local[dir]; ok {
			folder = &f
		}
	}
	if err := p.resolveProjectPaths(folder); err != nil {
		return nil, nil, fmt.Errorf("%s/%w", dir, err)
	}
	return p, ext, nil
}

func (p *Provider) operationNames() []string {
	names := make([]string, 0, len(p.Operations))
	for op := range p.Operations {
		names = append(names, op)
	}
	slices.Sort(names)
	return names
}

// nodeIs reports whether node holds a value of the config type typ.
func nodeIs(node *yaml.Node, typ string) bool {
	switch typ {
	case "map":
		return node.Kind == yaml.MappingNode
	case "list":
		return node.Kind == yaml.SequenceNode
	case "int":
		return node.Kind == yaml.ScalarNode && node.ShortTag() == "!!int"
	case "bool":
		return node.Kind == yaml.ScalarNode && node.ShortTag() == "!!bool"
	default:
		return node.Kind == yaml.ScalarNode && node.ShortTag() != "!!null"
	}
}

func sortedNames(m map[string]*yaml.Node) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
