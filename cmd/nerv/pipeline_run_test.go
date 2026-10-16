package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
)

// stepCall is one step nerv pipeline run started, with its arguments.
type stepCall struct {
	step string
	args []string
}

// runFixture fakes every step: each prints its JSON document and returns
// its exit code (0 unless set in exit).
type runFixture struct {
	top   string
	calls []stepCall
	exit  map[string]int
}

func newRunFixture(t *testing.T) *runFixture {
	t.Helper()
	return &runFixture{top: t.TempDir(), exit: map[string]int{}}
}

func (f *runFixture) run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	opts := testOptions(t.TempDir())
	opts.Runner = &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"git -C " + f.top + " rev-parse --show-toplevel": {Stdout: f.top + "\n"},
		"git -C . rev-parse --show-toplevel":             {Stdout: f.top + "\n"},
	}}
	opts.PipelineStep = func(step string, args []string, stdout, stderr io.Writer, _ options) int {
		f.calls = append(f.calls, stepCall{step, args})
		fmt.Fprintf(stderr, "[pipeline:%s] working\n", step)
		code := f.exit[step]
		status := map[int]string{0: "passed", 1: "refused", 2: "failed", 130: "interrupted"}[code]
		doc := map[string]any{"step": step, "status": status}
		if code != 0 {
			doc["error"] = step + " went wrong"
		}
		data, _ := json.Marshal(doc)
		fmt.Fprintf(stdout, "%s\n", data)
		return code
	}
	var out, errOut bytes.Buffer
	code := run(append([]string{"pipeline", "run"}, args...), &out, &errOut, opts)
	return code, out.String(), errOut.String()
}

func (f *runFixture) steps() []string {
	var out []string
	for _, c := range f.calls {
		out = append(out, c.step)
	}
	return out
}

func readRunJSON(t *testing.T, top string) (pipeline.RunResult, bool) {
	t.Helper()
	data, err := os.ReadFile(pipeline.RunResultPath(top))
	if err != nil {
		return pipeline.RunResult{}, false
	}
	var r pipeline.RunResult
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	return r, true
}

func outcomes(r pipeline.RunResult) []string {
	var out []string
	for _, s := range r.Steps {
		out = append(out, fmt.Sprintf("%s:%s:%d", s.Step, s.Status, s.Exit))
	}
	return out
}

func TestPipelineRun_RunsEveryStepInDimsOrder(t *testing.T) {
	f := newRunFixture(t)
	code, stdout, stderr := f.run(t, "--yes", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	if !slices.Equal(f.steps(), []string{"validate", "test", "release", "docker", "sync", "deploy"}) {
		t.Errorf("steps %q", f.steps())
	}
	want := map[string][]string{
		"validate": {"--cwd", f.top, "--json"},
		"test":     {"--cwd", f.top, "--json"},
		"release":  {"--cwd", f.top, "--json", "--yes"},
		"docker":   {"--cwd", f.top, "--json"},
		"sync":     {"--cwd", f.top, "--json", "--yes"},
		"deploy":   {"--cwd", f.top, "--json", "--yes"},
	}
	for _, c := range f.calls {
		if !slices.Equal(c.args, want[c.step]) {
			t.Errorf("%s args %q, want %q", c.step, c.args, want[c.step])
		}
	}
	var doc pipeline.RunResult
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("%v: %s", err, stdout)
	}
	r, ok := readRunJSON(t, f.top)
	if !ok || r.Status != "passed" || len(r.Steps) != 6 || doc.Status != "passed" || r.Steps[5].Status != "passed" {
		t.Errorf("run.json %+v ok %v, stdout %+v", r, ok, doc)
	}
	if !strings.Contains(stderr, "[pipeline:run] validate") || !strings.Contains(stderr, "[pipeline:test] working") {
		t.Errorf("stderr:\n%s", stderr)
	}
}

func TestPipelineRun_StopsAtTheFirstFailureWithItsExit(t *testing.T) {
	for _, tt := range []struct {
		fail string
		code int
		want []string
	}{
		{"test", 2, []string{"validate:passed:0", "test:failed:2", "release:not-run:0", "docker:not-run:0", "sync:not-run:0", "deploy:not-run:0"}},
		{"release", 1, []string{"validate:passed:0", "test:passed:0", "release:refused:1", "docker:not-run:0", "sync:not-run:0", "deploy:not-run:0"}},
		{"docker", 130, []string{"validate:passed:0", "test:passed:0", "release:passed:0", "docker:interrupted:130", "sync:not-run:0", "deploy:not-run:0"}},
	} {
		t.Run(tt.fail, func(t *testing.T) {
			f := newRunFixture(t)
			f.exit[tt.fail] = tt.code
			code, _, stderr := f.run(t, "--yes")
			if code != tt.code {
				t.Errorf("exit %d, want %d\n%s", code, tt.code, stderr)
			}
			r, ok := readRunJSON(t, f.top)
			if !ok || r.Status != map[int]string{1: "refused", 2: "failed", 130: "interrupted"}[tt.code] || !slices.Equal(outcomes(r), tt.want) {
				t.Errorf("run.json %q status %s", outcomes(r), r.Status)
			}
			if !strings.Contains(r.Error, tt.fail+" went wrong") || !strings.Contains(stderr, tt.fail+" went wrong") {
				t.Errorf("error %q\n%s", r.Error, stderr)
			}
		})
	}
}

func TestPipelineRun_DryRunPassesThrough(t *testing.T) {
	f := newRunFixture(t)
	if code, _, stderr := f.run(t, "--dry-run"); code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	for _, c := range f.calls {
		wantDry := c.step != "validate" && c.step != "test"
		if slices.Contains(c.args, "--dry-run") != wantDry || slices.Contains(c.args, "--yes") {
			t.Errorf("%s args %q", c.step, c.args)
		}
	}
	if r, _ := readRunJSON(t, f.top); !r.DryRun {
		t.Errorf("run.json %+v", r)
	}
}

func TestPipelineRun_FromAndOnly(t *testing.T) {
	f := newRunFixture(t)
	if code, _, _ := f.run(t, "--from", "docker", "--yes"); code != 0 || !slices.Equal(f.steps(), []string{"docker", "sync", "deploy"}) {
		t.Errorf("--from docker: exit %d steps %q", code, f.steps())
	}
	g := newRunFixture(t)
	if code, _, _ := g.run(t, "--only", "sync", "--yes"); code != 0 || !slices.Equal(g.steps(), []string{"sync"}) {
		t.Errorf("--only sync: exit %d steps %q", code, g.steps())
	}
	if r, _ := readRunJSON(t, g.top); len(r.Steps) != 1 {
		t.Errorf("run.json lists only the selected steps: %+v", r.Steps)
	}
}

func TestPipelineRun_Usage(t *testing.T) {
	for _, args := range [][]string{{"--from", "nope"}, {"--only", "x"}, {"--from", "docker", "--only", "sync"}, {"extra"}} {
		f := newRunFixture(t)
		code, _, stderr := f.run(t, args...)
		if code != 2 || len(f.calls) != 0 || !strings.Contains(stderr, "Usage: nerv pipeline") {
			t.Errorf("%q: exit %d calls %d\n%s", args, code, len(f.calls), stderr)
		}
	}
}

func TestPipelineRun_RealStepsStopAtTheFirstRefusal(t *testing.T) {
	// With the real step implementations: no pipeline: section, so validate
	// refuses and nothing else runs.
	f := newPipelineFixture(t, "enabled: true\n")
	f.runner.Responses["git -C "+f.top+" rev-parse --show-toplevel"] = envtest.Response{Stdout: f.top + "\n"}
	opts := testOptions(t.TempDir())
	opts.Runner = f.runner
	var out, errOut bytes.Buffer
	code := run([]string{"pipeline", "run", "--json"}, &out, &errOut, opts)
	var r pipeline.RunResult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if code != 1 || r.Status != "refused" || !slices.Equal(outcomes(r), []string{"validate:refused:1", "test:not-run:0", "release:not-run:0", "docker:not-run:0", "sync:not-run:0", "deploy:not-run:0"}) {
		t.Errorf("exit %d run %q\n%s", code, outcomes(r), errOut.String())
	}
	if !strings.Contains(r.Error, "no pipeline: section") {
		t.Errorf("error %q", r.Error)
	}
}

func TestPipelineRun_ExpectGoesToTheReleaseStep(t *testing.T) {
	f := newRunFixture(t)
	if code, _, stderr := f.run(t, "--yes", "--expect", "api=1.4.0", "--expect", "api2=2.0.0"); code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	for _, c := range f.calls {
		has := slices.Contains(c.args, "--expect")
		if want := c.step == "release"; has != want {
			t.Errorf("%s args %q", c.step, c.args)
		}
		if c.step == "release" && !slices.Equal(c.args[len(c.args)-4:], []string{"--expect", "api=1.4.0", "--expect", "api2=2.0.0"}) {
			t.Errorf("release args %q", c.args)
		}
	}
}
