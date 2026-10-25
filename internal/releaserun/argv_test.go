package releaserun

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
)

func TestRun_ExecRunsTheArgvWithoutAShell(t *testing.T) {
	root := repo(t, "src/api/.keep")
	f := &fakeProc{}
	r := newRunner(root, f)
	step := Step{
		Name:    "dotnet:build",
		Stage:   "build",
		Exec:    []string{"dotnet", "build", "My App.sln", "-p:Version=1.2.0"},
		Command: releaseconfig.Command{Workdir: "src/api", Env: map[string]string{"DOTNET_NOLOGO": "1"}},
	}
	if err := r.Run(context.Background(), step, Vars{Project: "api"}); err != nil {
		t.Fatalf("Run = %v", err)
	}
	if f.got.Name != filepath.Join("/bin", "dotnet") {
		t.Errorf("program = %q, want the looked-up dotnet", f.got.Name)
	}
	if want := []string{"build", "My App.sln", "-p:Version=1.2.0"}; !slices.Equal(f.got.Args, want) {
		t.Errorf("args = %q, want %q (no shell, no quoting)", f.got.Args, want)
	}
	if f.got.Dir != filepath.Join(root, "src", "api") {
		t.Errorf("dir = %q", f.got.Dir)
	}
	for _, kv := range []string{"DOTNET_NOLOGO=1", "NERV_RELEASE_STAGE=build", "NERV_RELEASE_PROJECT=api"} {
		if !slices.Contains(f.got.Env, kv) {
			t.Errorf("env lacks %s", kv)
		}
	}
}

func TestRun_StageDefaultsToTheStepName(t *testing.T) {
	f := &fakeProc{}
	r := newRunner(repo(t), f)
	if err := r.Run(context.Background(), Step{Name: "before_test", Command: releaseconfig.Command{Run: "true"}}, Vars{Project: "api"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.got.Env, "NERV_RELEASE_STAGE=before_test") {
		t.Errorf("NERV_RELEASE_STAGE not the step name: %q", f.got.Env)
	}
}

func TestRun_ExecFailureNamesTheProgramNotItsArguments(t *testing.T) {
	f := &fakeProc{handle: func(context.Context, proc.Command) (proc.Result, error) {
		return proc.Result{Stdout: []byte("error: 401\n")}, exitErr{code: 1, msg: "exit status 1"}
	}}
	r := newRunner(repo(t), f)
	err := r.Run(context.Background(), Step{Name: "nuget:push", Stage: "publish", Exec: []string{"dotnet", "nuget", "push", "--api-key", "s3cr3t"}}, Vars{Project: "api"})
	var se *StepError
	if !errors.As(err, &se) {
		t.Fatalf("Run = %v, want a *StepError", err)
	}
	if se.ExitCode != 1 || se.Shell != "exec" || se.Script != "dotnet" {
		t.Errorf("StepError = %+v, want exit 1, shell exec, program dotnet", se)
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("error shows an argument: %v", err)
	}
}

func TestRun_ExecMissingProgram(t *testing.T) {
	f := &fakeProc{}
	r := newRunner(repo(t), f)
	r.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	err := r.Run(context.Background(), Step{Name: "docker:build", Exec: []string{"docker", "build", "."}}, Vars{Project: "api"})
	if !errors.Is(err, ErrInterpreterNotFound) || !strings.Contains(err.Error(), "docker not found on PATH") {
		t.Fatalf("Run = %v, want docker not found", err)
	}
	if f.got.Name != "" {
		t.Errorf("a process was started: %q", f.got.Name)
	}
}
