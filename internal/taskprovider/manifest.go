package taskprovider

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
)

// ManifestFile is the manifest's name at the root of a provider plugin.
const ManifestFile = extension.ProviderFile

// ConfigScopes and ConfigTypes are the common config key schema's.
var (
	ConfigScopes = extension.ConfigScopes
	ConfigTypes  = extension.ConfigTypes
)

var prefixRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)

// ConfigKey is one setting the provider needs, with what the configure
// wizard shows for it.
type ConfigKey = extension.ConfigKey

// MCP names the server and the exact tools the provider calls, so the
// dispatcher can tell a missing connector from a provider failure.
type MCP = extension.MCP

// Manifest is a task provider's provider.yaml, as the task extension reads
// it. Its config keys are read from tasks.providers.<name>.<key> in the
// configuration files.
type Manifest struct {
	Name            string
	ContractVersion int
	TaskRefPrefix   string
	MCP             MCP
	Config          []ConfigKey
	Stages          map[string]string
	// Operations maps every operation to its procedure file, or to
	// NotImplemented.
	Operations map[string]string
	Events     []string
}

// Load reads and validates dir/provider.yaml from fsys (the extensions
// tree): the common contract against the task extension's
// extension.yaml, then the task fields.
func Load(fsys fs.FS, dir string) (*Manifest, error) {
	p, ext, err := extension.LoadProvider(fsys, dir)
	if err != nil {
		return nil, err
	}
	if ext.Name != Extension {
		return nil, fmt.Errorf("%s: a provider of extension %s, not a task provider", dir, ext.Name)
	}
	m, err := fromProvider(p)
	if err != nil {
		return nil, fmt.Errorf("%s/%w", dir, err)
	}
	return m, nil
}

// Parse decodes and validates a task provider.yaml against the task
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
	m := &Manifest{Name: p.Name, ContractVersion: p.ContractVersion, MCP: p.MCP, Config: p.Config, Operations: map[string]string{}}
	var problems []string
	for _, f := range []struct {
		name string
		v    any
	}{{"task_ref_prefix", &m.TaskRefPrefix}, {"stages", &m.Stages}, {"events", &m.Events}} {
		if err := p.DecodeField(f.name, f.v); err != nil {
			problems = append(problems, err.Error())
		}
	}
	for op, o := range p.Operations {
		m.Operations[op] = o.Procedure
		if o.NotImplemented {
			m.Operations[op] = NotImplemented
		}
	}
	if p.Runtime != extension.RuntimeAgent {
		problems = append(problems, fmt.Sprintf("runtime %q: the task dispatcher runs agent providers only", p.Runtime))
	}
	problems = append(problems, m.validate()...)
	if len(problems) > 0 {
		return nil, errors.New(ManifestFile + ": " + strings.Join(problems, "; "))
	}
	return m, nil
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

// validate checks the task fields: the task ref prefix, the canonical
// stage keys, and the events, each needing operations the provider
// implements.
func (m *Manifest) validate() []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }

	if !prefixRe.MatchString(m.TaskRefPrefix) {
		add("task_ref_prefix %q must be upper-case letters and digits", m.TaskRefPrefix)
	}
	for _, key := range sortedKeys(m.Stages) {
		if !slices.Contains(StageKeys, key) {
			add("stages.%s is not a canonical stage (%s)", key, strings.Join(StageKeys, ", "))
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

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
