package releaseplan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
)

// ErrNotImplemented is returned by Providers.Call for an optional
// operation the provider declares not_implemented.
var ErrNotImplemented = errors.New("not implemented by the provider")

// Providers calls the release extension's slot providers: the
// release-<name> providers of the extensions tree, each an exec
// provider whose steps get the common request on their stdin and answer
// the common response on their stdout.
type Providers struct {
	// FS is the extensions tree (nerv.ExtensionsFS()).
	FS fs.FS
	// Runner runs the steps; nil means proc.ExecRunner.
	Runner proc.Runner
	// Nerv is {nerv}; empty means os.Executable.
	Nerv string
	// Dir is the steps' working directory; empty means nerv's.
	Dir string
	// Output receives the steps' stderr live; nil only captures it.
	Output io.Writer
}

// Provider loads the provider name of slot (the slot's default when name
// is empty) and checks that the release extension can run it: an exec
// provider of that slot whose steps are literals and {nerv}. at names the
// key that selected it (release.build), for the refusal of an unknown
// one, which lists the slot's providers.
func (p Providers) Provider(slot, name, at string) (*extension.Provider, error) {
	ext, err := extension.LoadManifest(p.FS, Extension)
	if err != nil {
		return nil, err
	}
	s, ok := ext.Slot(slot)
	if !ok {
		return nil, fmt.Errorf("the release extension has no %s slot", slot)
	}
	if name == "" {
		name = s.Default
	}
	if name == "" {
		return nil, &releaseconfig.ValidationError{Problems: []string{fmt.Sprintf("%s: required (%s)", at, orList(p.SlotProviders(slot)))}}
	}
	dir := Extension + "-" + name
	if _, err := fs.Stat(p.FS, dir+"/"+extension.ProviderFile); err != nil || !extension.ValidName(name) {
		return nil, &releaseconfig.ValidationError{Problems: []string{fmt.Sprintf("%s: unknown %s provider %q (want %s)", at, slot, name, orList(p.SlotProviders(slot)))}}
	}
	prov, _, err := extension.LoadProvider(p.FS, dir)
	if err != nil {
		return nil, err
	}
	if prov.Slot != slot {
		return nil, &releaseconfig.ValidationError{Problems: []string{fmt.Sprintf("%s: %q is a %s provider, not a %s one (want %s)", at, name, prov.Slot, slot, orList(p.SlotProviders(slot)))}}
	}
	if err := checkSteps(prov); err != nil {
		return nil, fmt.Errorf("%s/%s: %w", dir, extension.ProviderFile, err)
	}
	return prov, nil
}

// SlotProviders are the names of the providers the tree ships for slot,
// in name order.
func (p Providers) SlotProviders(slot string) []string {
	entries, err := fs.ReadDir(p.FS, ".")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		name, ok := strings.CutPrefix(e.Name(), Extension+"-")
		if !ok || !e.IsDir() {
			continue
		}
		if prov, _, err := extension.LoadProvider(p.FS, e.Name()); err == nil && prov.Slot == slot {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// checkSteps refuses a step the release extension cannot run: it has no
// step variables but {nerv}, and no for_each, when or absent_stderr.
func checkSteps(prov *extension.Provider) error {
	if prov.Runtime != extension.RuntimeExec {
		return fmt.Errorf("runtime %q: nerv release runs exec providers only", prov.Runtime)
	}
	var problems []string
	for op, o := range prov.Operations {
		for i, s := range o.Steps {
			at := fmt.Sprintf("operations.%s[%d]", op, i)
			if s.ForEach != "" || s.When != "" || s.AbsentStderr != "" {
				problems = append(problems, at+": for_each, when and absent_stderr are not release step fields")
			}
			for j, a := range s.Argv {
				if a.When != "" || (strings.Contains(a.Value, "{") && a.Value != "{nerv}") {
					problems = append(problems, fmt.Sprintf("%s.argv[%d] %q: a release step has no variable but {nerv}", at, j, a.Value))
				}
			}
		}
	}
	if len(problems) > 0 {
		slices.Sort(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// Call runs op of prov with config and payload, and decodes the data of
// the last response into data (when not nil). A not_implemented
// operation is ErrNotImplemented. The last response is returned, also on
// error: a provider that answered failed, blocked or needs_input is an
// error carrying its message.
func (p Providers) Call(ctx context.Context, prov *extension.Provider, op string, config map[string]any, payload, data any) (extension.Response, error) {
	o, ok := prov.Operations[op]
	if !ok || o.NotImplemented {
		return extension.Response{}, fmt.Errorf("provider %s: %s: %w", prov.Name, op, ErrNotImplemented)
	}
	nerv := p.Nerv
	if nerv == "" {
		var err error
		if nerv, err = os.Executable(); err != nil {
			return extension.Response{}, err
		}
	}
	if config == nil {
		config = map[string]any{}
	}
	req := extension.Request{
		Contract: extension.ContractVersion, Extension: Extension, Slot: prov.Slot, Provider: prov.Name, Operation: op,
		Config: config, Payload: payload,
	}
	var last extension.Response
	for _, s := range o.Steps {
		argv := make([]string, len(s.Argv))
		for i, a := range s.Argv {
			argv[i] = a.Value
			if a.Value == "{nerv}" {
				argv[i] = nerv
			}
		}
		resp, _, err := extension.Exec(ctx, p.Runner, "provider "+prov.Name+": "+op, extension.Call{Argv: argv, Dir: p.Dir, Stderr: p.Output}, req)
		last = resp
		if err != nil {
			return resp, err
		}
	}
	if data != nil {
		if err := last.DecodeData(data); err != nil {
			return last, fmt.Errorf("provider %s: %s: data: %w", prov.Name, op, err)
		}
	}
	return last, nil
}

// validate runs prov's validate, when it implements it: a failure is a
// *releaseconfig.ValidationError of its problems (or of its message).
func (p Providers) validate(ctx context.Context, prov *extension.Provider, config map[string]any, payload any) error {
	if !prov.Implements(OpValidate) {
		return nil
	}
	var problems ProblemsData
	resp, err := p.Call(ctx, prov, OpValidate, config, payload, nil)
	if err == nil {
		return nil
	}
	if resp.Status == "" {
		return err
	}
	if derr := resp.DecodeData(&problems); derr == nil && len(problems.Problems) > 0 {
		return &releaseconfig.ValidationError{Problems: problems.Problems}
	}
	if resp.Message != "" {
		return &releaseconfig.ValidationError{Problems: []string{resp.Message}}
	}
	return err
}

// providerConfig is a provider's config: the keys of each layer (a
// section-wide one, then a project's), checked against the provider's
// config keys, each problem named by its layer's path.
func providerConfig(prov *extension.Provider, layers ...configLayer) (map[string]any, []string) {
	var problems []string
	values := make([]map[string]any, len(layers))
	for i, l := range layers {
		_, p := extension.ResolveConfig(prov.Config, l.at, l.values)
		problems = append(problems, p...)
		values[i] = l.values
	}
	if len(problems) > 0 {
		return nil, problems
	}
	cfg, _ := extension.ResolveConfig(prov.Config, "", values...)
	return cfg, nil
}

// configLayer is one place a provider's keys are set.
type configLayer struct {
	at     string
	values map[string]any
}

func orList(names []string) string {
	if len(names) == 0 {
		return "none is shipped"
	}
	return strings.Join(names, ", ")
}
