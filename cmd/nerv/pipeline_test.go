package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

const pipelineYAML = `pipeline:
  dotnet:
    solution: App.slnx
  images:
    - name: ghcr.io/acme/app
`

type pipelineFixture struct {
	top    string
	runner *envtest.FakeRunner
	steps  []shiprun.Step
	fail   map[string]error
	root   string
	cancel context.CancelFunc
	signal os.Signal
	onRun  func()
}

func newPipelineFixture(t *testing.T, yaml string) *pipelineFixture {
	t.Helper()
	top := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(top, ".nerv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if yaml != "" {
		if err := os.WriteFile(filepath.Join(top, ".nerv", "nerv.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &pipelineFixture{top: top, runner: &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"git -C . rev-parse --show-toplevel": {Stdout: top + "\n"},
	}}}
}

type pipelineRecorder struct{ f *pipelineFixture }

func (r pipelineRecorder) Run(_ context.Context, s shiprun.Step, _ shiprun.Vars) error {
	if r.f.onRun != nil {
		r.f.onRun()
	}
	r.f.steps = append(r.f.steps, s)
	return r.f.fail[strings.Join(s.Exec[:2], " ")]
}

func (f *pipelineFixture) run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	opts := testOptions(t.TempDir())
	opts.Runner = f.runner
	opts.PipelineSteps = func(root string, _ io.Writer) pipeline.StepRunner {
		f.root = root
		return pipelineRecorder{f}
	}
	opts.SignalContext = func(parent context.Context) (context.Context, context.CancelFunc, func() os.Signal) {
		ctx, cancel := context.WithCancel(parent)
		f.cancel = cancel
		return ctx, cancel, func() os.Signal { return f.signal }
	}
	var out, errOut bytes.Buffer
	code := run(append([]string{"pipeline"}, args...), &out, &errOut, opts)
	return code, out.String(), errOut.String()
}

func TestPipeline_Usage(t *testing.T) {
	f := newPipelineFixture(t, pipelineYAML)
	for _, args := range [][]string{nil, {"deploy"}, {"validate", "extra"}, {"test", "--nope"}} {
		code, _, stderr := f.run(t, args...)
		if code != 2 || !strings.Contains(stderr, "Usage: nerv pipeline <step>") || !strings.Contains(stderr, "validate") || !strings.Contains(stderr, "test") {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
	if code, stdout, _ := f.run(t, "--help"); code != 0 || !strings.Contains(stdout, "working tree") {
		t.Errorf("--help: exit %d %q", code, stdout)
	}
	if len(f.steps) != 0 {
		t.Errorf("ran %v", f.steps)
	}
}

func TestPipeline_NoOrInvalidSectionIsARefusal(t *testing.T) {
	tests := []struct{ name, yaml, want string }{
		{"no section", "enabled: true\n", "docs/pipeline.md"},
		{"invalid", "pipeline:\n  images:\n    - name: ghcr.io/acme/app\n", "pipeline.dotnet.solution"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPipelineFixture(t, tt.yaml)
			code, stdout, stderr := f.run(t, "validate", "--json")
			var doc pipeline.Result
			if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
				t.Fatalf("stdout: %v %q", err, stdout)
			}
			if code != 1 || doc.Status != "refused" || doc.Step != "validate" || !strings.Contains(doc.Error, tt.want) || !strings.Contains(stderr, tt.want) || len(f.steps) != 0 {
				t.Errorf("exit %d, %+v, stderr %s", code, doc, stderr)
			}
		})
	}
}

func TestPipeline_ValidateRunsInTheWorkingTree(t *testing.T) {
	f := newPipelineFixture(t, pipelineYAML)
	code, stdout, stderr := f.run(t, "validate", "--cwd", ".")
	if code != 0 || f.root != f.top || len(f.steps) != 2 || f.steps[1].Exec[1] != "format" {
		t.Fatalf("exit %d, root %s, steps %v\n%s", code, f.root, f.steps, stderr)
	}
	if !strings.Contains(stdout, "validate: passed") {
		t.Errorf("summary %q", stdout)
	}
}

func TestPipeline_TestJSONDocument(t *testing.T) {
	f := newPipelineFixture(t, pipelineYAML)
	f.fail = map[string]error{"dotnet test": &shiprun.StepError{ExitCode: 1, Reason: "exit code 1"}}
	code, stdout, _ := f.run(t, "test", "--json")
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	if code != 2 || doc["step"] != "test" || doc["status"] != "failed" || doc["error"] == "" {
		t.Errorf("exit %d, %v", code, doc)
	}
	cmds, _ := doc["commands"].([]any)
	last, _ := cmds[len(cmds)-1].(map[string]any)
	if len(cmds) != 3 || last["exit"] != float64(1) {
		t.Errorf("commands %v", cmds)
	}
	arts, _ := doc["artifacts"].(map[string]any)
	if arts["results_dir"] != pipeline.ResultsDir(f.top) {
		t.Errorf("artifacts %v", arts)
	}
}

func TestPipeline_TextReportsWhereTheResultsAre(t *testing.T) {
	f := newPipelineFixture(t, pipelineYAML)
	code, stdout, _ := f.run(t, "test")
	if code != 0 || !strings.Contains(stdout, "test: passed") || !strings.Contains(stdout, pipeline.ResultsDir(f.top)) {
		t.Errorf("exit %d %q", code, stdout)
	}
}

func TestPipeline_Interrupted(t *testing.T) {
	f := newPipelineFixture(t, pipelineYAML)
	f.signal = syscall.SIGINT
	f.onRun = func() { f.cancel() }
	f.fail = map[string]error{"dotnet restore": &shiprun.StepError{ExitCode: -1, Reason: "interrupted", Err: errors.New("context canceled")}}
	code, stdout, stderr := f.run(t, "validate", "--json")
	var doc pipeline.Result
	_ = json.Unmarshal([]byte(stdout), &doc)
	if code != 130 || doc.Status != "interrupted" || !strings.Contains(stderr, "interrupted") {
		t.Errorf("exit %d, %+v, %s", code, doc, stderr)
	}
}
