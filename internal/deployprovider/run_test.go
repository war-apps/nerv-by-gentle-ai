package deployprovider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// fakeProc records every command and answers from a script.
type fakeProc struct {
	calls  []proc.Command
	answer func(c proc.Command) (proc.Result, error)
}

func (f *fakeProc) Run(_ context.Context, c proc.Command) (proc.Result, error) {
	f.calls = append(f.calls, c)
	if f.answer == nil {
		return proc.Result{}, nil
	}
	return f.answer(c)
}

type exitErr int

func (e exitErr) Error() string { return "exit status " + string(rune('0'+int(e))) }
func (e exitErr) ExitCode() int { return int(e) }

func mustParse(t *testing.T, s string) *Manifest {
	t.Helper()
	m, err := Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func sampleEnv(prerelease bool) Env {
	values := map[string]string{
		"project": "api", "version": "1.2.0", "tag": "v1.2.0", "notes_file": "/a/notes.md; rm -rf /",
		"prerelease": "false", "nerv": "/usr/bin/nerv", "config.repo": "acme/api",
	}
	if prerelease {
		values["prerelease"] = "true"
	}
	return Env{
		Values:  values,
		Lists:   map[string][]string{"images": {"r/api:1.2.0", "r/web:1.2.0"}, "image_tars": {"/a/outputs/api.tar"}},
		Request: Request{Artifact: map[string]string{"version": "1.2.0"}, Config: map[string]string{"repo": "acme/api"}},
	}
}

// A variable is substituted whole, as one argument, whatever it holds; a
// conditional argument is kept only when its variable is true.
func TestCommands_SubstituteWholeArguments(t *testing.T) {
	m := mustParse(t, validHost)
	got, err := m.Commands(OpCreateRelease, sampleEnv(false))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"acme", "create", "v1.2.0", "--notes-file", "/a/notes.md; rm -rf /"}
	if len(got) != 1 || strings.Join(got[0], "\x00") != strings.Join(want, "\x00") {
		t.Errorf("commands %q, want %q", got, want)
	}
	got, _ = m.Commands(OpCreateRelease, sampleEnv(true))
	if last := got[0][len(got[0])-1]; last != "--prerelease" {
		t.Errorf("prerelease argv %q", got[0])
	}
}

func TestCommands_RunForEachItem(t *testing.T) {
	m := mustParse(t, validTarget)
	got, err := m.Commands(OpPublish, sampleEnv(false))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"docker", "load", "-i", "/a/outputs/api.tar"}, {"docker", "push", "r/api:1.2.0"}, {"docker", "push", "r/web:1.2.0"}}
	if len(got) != len(want) {
		t.Fatalf("commands %q, want %q", got, want)
	}
	for i := range want {
		if strings.Join(got[i], " ") != strings.Join(want[i], " ") {
			t.Errorf("command %d %q, want %q", i, got[i], want[i])
		}
	}
	got, _ = m.Commands(OpDeploy, sampleEnv(false))
	if got[0][0] != "/usr/bin/nerv" {
		t.Errorf("{nerv} gave %q", got[0])
	}
}

// Each step reads the JSON request on stdin; a JSON response on stdout
// gives the status; a non-zero exit or a failed status is an error.
func TestRun_SendsTheRequestAndReadsTheResponse(t *testing.T) {
	m := mustParse(t, validTarget)
	f := &fakeProc{answer: func(c proc.Command) (proc.Result, error) {
		if c.Args[0] == "deploy-box" {
			return proc.Result{Stdout: []byte(`{"status":"skipped","message":"disabled"}`)}, nil
		}
		return proc.Result{Stdout: []byte("Loaded image\n")}, nil
	}}
	out, err := m.Run(context.Background(), f, OpDeploy, sampleEnv(false))
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != StatusSkipped || out.Message != "disabled" {
		t.Errorf("outcome %+v", out)
	}
	var req Request
	if err := json.Unmarshal(f.calls[0].Stdin, &req); err != nil {
		t.Fatal(err)
	}
	if req.Operation != OpDeploy || req.ContractVersion != 1 || req.Config["repo"] != "acme/api" {
		t.Errorf("request %+v", req)
	}

	out, err = m.Run(context.Background(), f, OpPublish, sampleEnv(false))
	if err != nil || out.Status != StatusOK || len(f.calls) != 4 {
		t.Errorf("publish: %+v %v, %d calls", out, err, len(f.calls))
	}
	var item Request
	_ = json.Unmarshal(f.calls[3].Stdin, &item)
	if item.Item != "r/web:1.2.0" {
		t.Errorf("for_each request item %q", item.Item)
	}

	f.answer = func(proc.Command) (proc.Result, error) {
		return proc.Result{Stdout: []byte(`{"status":"failed","message":"unhealthy"}`)}, nil
	}
	if _, err := m.Run(context.Background(), f, OpDeploy, sampleEnv(false)); err == nil || !strings.Contains(err.Error(), "unhealthy") {
		t.Errorf("failed status: %v", err)
	}
	f.answer = func(proc.Command) (proc.Result, error) {
		return proc.Result{Stderr: []byte("denied: push access\n")}, exitErr(1)
	}
	if _, err := m.Run(context.Background(), f, OpPublish, sampleEnv(false)); err == nil || !strings.Contains(err.Error(), "denied: push access") {
		t.Errorf("non-zero exit: %v", err)
	}
	if _, err := m.Run(context.Background(), f, OpVerify, sampleEnv(false)); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("not implemented: %v", err)
	}
}

func TestReleaseExists_ReadsTheExitCodeAndAbsentStderr(t *testing.T) {
	m := mustParse(t, validHost)
	f := &fakeProc{}
	if ok, err := m.ReleaseExists(context.Background(), f, sampleEnv(false)); err != nil || !ok {
		t.Errorf("exit 0: %v %v", ok, err)
	}
	f.answer = func(proc.Command) (proc.Result, error) {
		return proc.Result{Stderr: []byte("release not found\n")}, exitErr(1)
	}
	if ok, err := m.ReleaseExists(context.Background(), f, sampleEnv(false)); err != nil || ok {
		t.Errorf("absent: %v %v", ok, err)
	}
	f.answer = func(proc.Command) (proc.Result, error) {
		return proc.Result{Stderr: []byte("HTTP 401: Bad credentials\n")}, exitErr(1)
	}
	if _, err := m.ReleaseExists(context.Background(), f, sampleEnv(false)); err == nil || !strings.Contains(err.Error(), "Bad credentials") {
		t.Errorf("other failure: %v", err)
	}
	f.answer = func(proc.Command) (proc.Result, error) {
		return proc.Result{Stdout: []byte(`{"status":"ok","exists":false}`)}, nil
	}
	if ok, err := m.ReleaseExists(context.Background(), f, sampleEnv(false)); err != nil || ok {
		t.Errorf("JSON exists false: %v %v", ok, err)
	}
}
