package extension_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// stepRunner answers every command with res and err, and records them.
type stepRunner struct {
	calls []proc.Command
	res   proc.Result
	err   error
}

func (s *stepRunner) Run(_ context.Context, c proc.Command) (proc.Result, error) {
	s.calls = append(s.calls, c)
	return s.res, s.err
}

type exitCode int

func (e exitCode) Error() string { return "exit status 1" }
func (e exitCode) ExitCode() int { return int(e) }

func execReq() extension.Request {
	return extension.Request{Contract: extension.ContractVersion, Extension: "acme", Slot: "box", Provider: "tin", Operation: "ship",
		Config: map[string]any{"region": "eu"}, Payload: map[string]string{"target": "staging"}}
}

// Exec sends the request on stdin, runs the argv shell-free in Dir and
// answers the step's response.
func TestExec_SendsTheRequestAndReadsTheResponse(t *testing.T) {
	r := &stepRunner{res: proc.Result{Stdout: []byte(`{"status":"skipped","message":"disabled"}`)}}
	resp, _, err := extension.Exec(context.Background(), r, "provider tin: ship", extension.Call{Argv: []string{"tin", "ship"}, Dir: "/w"}, execReq())
	if err != nil || resp.Status != extension.StatusSkipped || resp.Message != "disabled" {
		t.Fatalf("Exec = %+v, %v", resp, err)
	}
	c := r.calls[0]
	if c.Name != "tin" || len(c.Args) != 1 || c.Args[0] != "ship" || c.Dir != "/w" {
		t.Errorf("command %+v", c)
	}
	var got extension.Request
	if err := json.Unmarshal(c.Stdin, &got); err != nil || got.Operation != "ship" || got.Config["region"] != "eu" {
		t.Errorf("stdin %s: %v", c.Stdin, err)
	}
}

// A failure carries where, the argv and the response's message, or the
// last stderr line when the step printed no response.
func TestExec_Failures(t *testing.T) {
	tests := []struct {
		name string
		res  proc.Result
		err  error
		want string
	}{
		{"failed status", proc.Result{Stdout: []byte(`{"status":"failed","message":"unhealthy"}`)}, nil, "provider tin: ship: tin ship: failed: unhealthy"},
		{"blocked status", proc.Result{Stdout: []byte(`{"status":"blocked","message":"locked"}`)}, nil, "blocked: locked"},
		{"needs_input status", proc.Result{Stdout: []byte(`{"status":"needs_input","message":"which?"}`)}, nil, "needs_input: which?"},
		{"non-zero exit with stderr", proc.Result{Stderr: []byte("one\ndenied\n")}, exitCode(1), "tin ship: exit status 1: denied"},
		{"non-zero exit with a message", proc.Result{Stdout: []byte(`{"status":"failed","message":"bad config"}`), Stderr: []byte("noise\n")}, exitCode(1), "exit status 1: bad config"},
		// An unknown status is not ok: the step speaks another contract.
		{"unknown status", proc.Result{Stdout: []byte(`{"status":"done","message":"shipped"}`)}, nil, `tin ship: unknown status "done" (want ok, skipped, blocked, failed or needs_input)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &stepRunner{res: tt.res, err: tt.err}
			_, _, err := extension.Exec(context.Background(), r, "provider tin: ship", extension.Call{Argv: []string{"tin", "ship"}}, execReq())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Exec error %v, want %q", err, tt.want)
			}
		})
	}
	// No response and exit 0 is ok; so is a response without a status.
	for _, out := range []string{"", "Loaded image\n", `{"message":"done"}`} {
		r := &stepRunner{res: proc.Result{Stdout: []byte(out)}}
		if _, _, err := extension.Exec(context.Background(), r, "x", extension.Call{Argv: []string{"tin"}}, execReq()); err != nil {
			t.Errorf("stdout %q: %v", out, err)
		}
	}
	if code := extension.ExitCode(errors.New("x")); code != -1 {
		t.Errorf("ExitCode without a code = %d", code)
	}
}

func TestValidTool(t *testing.T) {
	for _, ok := range []string{"gh", "docker-compose", "python3.12", "g++"} {
		if !extension.ValidTool(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "-rf", "../bin/x", "/usr/bin/gh", "a b"} {
		if extension.ValidTool(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
