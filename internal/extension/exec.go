package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// Call is one run of a rendered exec step: its argv (the command first),
// where it runs and where its stderr streams.
type Call struct {
	Argv []string
	// Dir is the working directory; Env the environment (nil inherits
	// nerv's).
	Dir string
	Env []string
	// Stderr receives the step's stderr live; nil only captures it.
	Stderr io.Writer
}

// Exec runs the step c with req as JSON on its stdin and reads the
// response it prints on its stdout. A non-zero exit is an error carrying
// the response's message or the last stderr line, and so is a response
// whose status is a Failure; where prefixes every error (such as
// "provider ssh-compose: deploy"). The returned Response is whatever the
// step printed (zero when it printed none), also on error.
func Exec(ctx context.Context, runner proc.Runner, where string, c Call, req Request) (Response, proc.Result, error) {
	if runner == nil {
		runner = proc.ExecRunner{}
	}
	if len(c.Argv) == 0 {
		return Response{}, proc.Result{}, fmt.Errorf("%s: no command", where)
	}
	stdin, err := json.Marshal(req)
	if err != nil {
		return Response{}, proc.Result{}, err
	}
	res, err := runner.Run(ctx, proc.Command{Name: c.Argv[0], Args: c.Argv[1:], Dir: c.Dir, Env: c.Env, Stdin: stdin, Stderr: c.Stderr})
	resp, _ := ParseResponse(res.Stdout)
	at := where + ": " + strings.Join(c.Argv, " ")
	if err != nil {
		detail := LastLine(res.Stderr)
		if resp.Message != "" {
			detail = resp.Message
		}
		if detail != "" {
			return resp, res, fmt.Errorf("%s: %w: %s", at, err, detail)
		}
		return resp, res, fmt.Errorf("%s: %w", at, err)
	}
	if resp.Failure() {
		return resp, res, fmt.Errorf("%s: %s: %s", at, resp.Status, resp.Message)
	}
	return resp, res, nil
}

// ExitCode is the exit code an Exec error carries, -1 when it carries
// none (the command could not start).
func ExitCode(err error) int {
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return -1
}

// LastLine is the last non-empty line of b, trimmed.
func LastLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}

// Answer writes resp as the one JSON line an exec step prints on its
// stdout, and returns the exit code that goes with it: 0 for ok and
// skipped, 1 otherwise. A native provider's operation ends with it.
func Answer(stdout io.Writer, resp Response) int {
	data, err := json.Marshal(resp)
	if err != nil {
		return 2
	}
	if _, err := fmt.Fprintf(stdout, "%s\n", data); err != nil {
		return 2
	}
	if resp.Failure() {
		return 1
	}
	return 0
}

// ReadRequest decodes the Request an exec step reads on its stdin.
func ReadRequest(stdin io.Reader) (Request, error) {
	var req Request
	data, err := io.ReadAll(stdin)
	if err != nil {
		return req, err
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return req, fmt.Errorf("the request on stdin is not the JSON envelope: %w", err)
	}
	if req.Contract != ContractVersion {
		return req, fmt.Errorf("request contract %d is not supported (this nerv understands %d)", req.Contract, ContractVersion)
	}
	return req, nil
}

// DecodePayload decodes the request's payload into v.
func (r Request) DecodePayload(v any) error { return recode(r.Payload, v) }

// DecodeConfig decodes the request's config into v.
func (r Request) DecodeConfig(v any) error { return recode(r.Config, v) }

func recode(from, to any) error {
	data, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, to)
}
