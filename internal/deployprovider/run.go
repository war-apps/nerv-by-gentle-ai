package deployprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// ErrNotImplemented is returned for an operation the provider declares
// not_implemented.
var ErrNotImplemented = errors.New("not implemented by the provider")

// Env is what an operation runs with.
type Env struct {
	// Values are the variables (Variables plus config.<key>).
	Values map[string]string
	// Lists are the for_each lists (ForEach).
	Lists map[string][]string
	// Config is the provider's resolved config, sent in every request.
	Config map[string]string
	// Payload is the base of every request's payload; Item is filled per
	// step.
	Payload Payload
	// Dir is the steps' working directory; ProcEnv their environment (nil
	// inherits nerv's).
	Dir     string
	ProcEnv []string
	// Output receives the steps' stderr live; nil captures it only.
	Output io.Writer
}

// Outcome is the result of an operation: StatusOK or StatusSkipped (a
// failure is an error), with the last message a step reported.
type Outcome struct {
	Status  string
	Message string
}

// command is one rendered step run.
type command struct {
	step Step
	argv []string
	item string
}

// Commands renders the commands op runs with env, in order (for a dry run
// or a log): steps whose when is false are left out, for_each steps give
// one command per item.
func (m *Manifest) Commands(op string, env Env) ([][]string, error) {
	cmds, err := m.render(op, env)
	if err != nil {
		return nil, err
	}
	out := make([][]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.argv
	}
	return out, nil
}

func (m *Manifest) render(op string, env Env) ([]command, error) {
	o, ok := m.Operations[op]
	if !ok || o.NotImplemented {
		return nil, fmt.Errorf("provider %s: %s: %w", m.Name, op, ErrNotImplemented)
	}
	var out []command
	for _, s := range o.Steps {
		if s.When != "" && env.Values[s.When] != "true" {
			continue
		}
		items := []string{""}
		if s.ForEach != "" {
			items = env.Lists[s.ForEach]
		}
		for _, item := range items {
			argv, err := renderArgv(s.Argv, env.Values, item)
			if err != nil {
				return nil, fmt.Errorf("provider %s: %s: %w", m.Name, op, err)
			}
			out = append(out, command{step: s, argv: argv, item: item})
		}
	}
	return out, nil
}

// renderArgv substitutes each {variable} whole; nothing is ever split or
// expanded.
func renderArgv(args []Arg, values map[string]string, item string) ([]string, error) {
	argv := make([]string, 0, len(args))
	for _, a := range args {
		if a.When != "" && values[a.When] != "true" {
			continue
		}
		v := a.Value
		if m := variableRe.FindStringSubmatch(v); m != nil {
			if m[1] == "item" {
				v = item
			} else {
				var ok bool
				if v, ok = values[m[1]]; !ok {
					return nil, fmt.Errorf("variable {%s} has no value", m[1])
				}
			}
		}
		argv = append(argv, v)
	}
	return argv, nil
}

// Run runs op's commands in order, stopping at the first failure: a
// non-zero exit or a response with status failed, blocked or needs_input.
// A skipped response makes the outcome skipped.
func (m *Manifest) Run(ctx context.Context, runner proc.Runner, op string, env Env) (Outcome, error) {
	cmds, err := m.render(op, env)
	if err != nil {
		return Outcome{}, err
	}
	out := Outcome{Status: StatusOK}
	for _, c := range cmds {
		resp, _, err := m.exec(ctx, runner, op, env, c)
		if err != nil {
			return out, err
		}
		if resp.Status == StatusSkipped {
			out.Status = StatusSkipped
		}
		if resp.Message != "" {
			out.Message = resp.Message
		}
	}
	return out, nil
}

// ReleaseExists runs the releaseExists step: a JSON exists answer wins;
// otherwise exit 0 means it exists, and a non-zero exit whose stderr
// matches absent_stderr means it does not.
func (m *Manifest) ReleaseExists(ctx context.Context, runner proc.Runner, env Env) (bool, error) {
	cmds, err := m.render(OpReleaseExists, env)
	if err != nil {
		return false, err
	}
	if len(cmds) != 1 {
		return false, fmt.Errorf("provider %s: %s: its step did not run", m.Name, OpReleaseExists)
	}
	c := cmds[0]
	// The check's stderr is not streamed: a missing release is an expected
	// answer (gh's "release not found"), and any other failure carries its
	// last line in the error.
	quiet := env
	quiet.Output = nil
	resp, res, err := m.exec(ctx, runner, OpReleaseExists, quiet, c)
	if err != nil {
		if pattern := c.step.AbsentStderr; pattern != "" && exitCode(err) > 0 && regexp.MustCompile(pattern).Match(res.Stderr) {
			return false, nil
		}
		return false, err
	}
	var data ExistsData
	if err := resp.DecodeData(&data); err != nil {
		return false, fmt.Errorf("provider %s: %s: data: %w", m.Name, OpReleaseExists, err)
	}
	if data.Exists != nil {
		return *data.Exists, nil
	}
	return true, nil
}

// Preflight runs a target's preflight, before anything is written, and
// returns the executables its config needs besides requires (the
// response's data.requires). A provider that does not implement it needs
// none. A refused config is an error carrying the provider's message.
func (m *Manifest) Preflight(ctx context.Context, runner proc.Runner, env Env) ([]string, error) {
	if !m.Implements(OpPreflight) {
		return nil, nil
	}
	cmds, err := m.render(OpPreflight, env)
	if err != nil {
		return nil, err
	}
	var requires []string
	for _, c := range cmds {
		quiet := env
		quiet.Output = nil
		resp, _, err := m.exec(ctx, runner, OpPreflight, quiet, c)
		if err != nil {
			if resp.Failure() && resp.Message != "" {
				return nil, errors.New(resp.Message)
			}
			return nil, err
		}
		var data PreflightData
		if err := resp.DecodeData(&data); err != nil {
			return nil, fmt.Errorf("provider %s: %s: data: %w", m.Name, OpPreflight, err)
		}
		for _, tool := range data.Requires {
			if !toolRe.MatchString(tool) {
				return nil, fmt.Errorf("provider %s: %s: %q is not an executable name", m.Name, OpPreflight, tool)
			}
			if !slices.Contains(requires, tool) {
				requires = append(requires, tool)
			}
		}
	}
	return requires, nil
}

var toolRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// exec runs one command with its request on stdin and reads its response.
func (m *Manifest) exec(ctx context.Context, runner proc.Runner, op string, env Env, c command) (extension.Response, proc.Result, error) {
	payload := env.Payload
	payload.Item = c.item
	config := make(map[string]any, len(env.Config))
	for k, v := range env.Config {
		config[k] = v
	}
	req := extension.Request{
		Contract: ContractVersion, Extension: Extension, Slot: m.Slot, Provider: m.Name, Operation: op,
		Config: config, Payload: payload,
	}
	stdin, err := json.Marshal(req)
	if err != nil {
		return extension.Response{}, proc.Result{}, err
	}
	pc := proc.Command{Name: c.argv[0], Args: c.argv[1:], Dir: env.Dir, Env: env.ProcEnv, Stdin: stdin, Stderr: env.Output}
	res, err := runner.Run(ctx, pc)
	resp, _ := extension.ParseResponse(res.Stdout)
	where := fmt.Sprintf("provider %s: %s: %s", m.Name, op, strings.Join(c.argv, " "))
	if err != nil {
		detail := lastLine(res.Stderr)
		if resp.Message != "" {
			detail = resp.Message
		}
		if detail != "" {
			return resp, res, fmt.Errorf("%s: %w: %s", where, err, detail)
		}
		return resp, res, fmt.Errorf("%s: %w", where, err)
	}
	if resp.Failure() {
		return resp, res, fmt.Errorf("%s: %s: %s", where, resp.Status, resp.Message)
	}
	return resp, res, nil
}

func exitCode(err error) int {
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return -1
}

func lastLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}
