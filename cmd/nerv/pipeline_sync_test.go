package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
)

const pipelineSyncYAML = "pipeline:\n  dotnet:\n    solution: App.slnx\n  images:\n    - name: ghcr.io/acme/api\n"

type fakeSyncWorkspace struct {
	origin    []string
	conflicts map[string]bool
	pushErr   error
	identErr  error
	opened    *semrel.WorkspaceOptions
	merges    []string
	pushes    []string
	previews  []string
	pushCheck int
	closed    bool
	onPush    func()
}

func (w *fakeSyncWorkspace) Branches(_ context.Context, pattern string) ([]string, error) {
	var out []string
	for _, b := range w.origin {
		if ok, _ := path.Match(pattern, b); ok {
			out = append(out, b)
		}
	}
	return out, nil
}

func (w *fakeSyncWorkspace) Merge(_ context.Context, target, source, message string) (semrel.MergeResult, error) {
	w.merges = append(w.merges, message)
	if w.conflicts[target] {
		return semrel.MergeResult{Conflict: true, Conflicts: []string{"CHANGELOG.md"}}, nil
	}
	return semrel.MergeResult{Commit: "abcdef0123456789"}, nil
}

func (w *fakeSyncWorkspace) PreviewMerge(_ context.Context, target, source string) (semrel.MergeResult, error) {
	w.previews = append(w.previews, source+">"+target)
	return semrel.MergeResult{Conflict: w.conflicts[target]}, nil
}

func (w *fakeSyncWorkspace) PushBranch(_ context.Context, branch string) error {
	w.pushes = append(w.pushes, branch)
	if w.onPush != nil {
		w.onPush()
	}
	return nil
}

func (w *fakeSyncWorkspace) CheckPush(context.Context) error { w.pushCheck++; return w.pushErr }
func (w *fakeSyncWorkspace) UseCommitIdentity(context.Context) (string, error) {
	return "Dev <dev@example.com>", w.identErr
}
func (w *fakeSyncWorkspace) Close() error { w.closed = true; return nil }

type syncFixture struct {
	top      string
	ws       *fakeSyncWorkspace
	terminal bool
	stdin    string
	cancel   context.CancelFunc
	signal   os.Signal
}

func newSyncFixture(t *testing.T, yaml string) *syncFixture {
	t.Helper()
	top := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(top, ".nerv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, ".ai-workflow.yml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return &syncFixture{top: top, ws: &fakeSyncWorkspace{origin: []string{"main", "develop", "release/a", "release/b"}}}
}

// release writes release.json and the passing records of the earlier
// steps bound to it: test.json at the release's base commit and docker.json
// for its version, tag and released commit.
func (f *syncFixture) release(t *testing.T, o pipeline.ReleaseOutputs) {
	t.Helper()
	if _, err := pipeline.WriteReleaseOutputs(f.top, o); err != nil {
		t.Fatal(err)
	}
	f.testRecord(t, pipeline.TestRecord{Step: "test", Status: "passed", Commit: o.BaseCommit})
	f.docker(t, pipeline.DockerResult{Step: "docker", Status: "passed", DryRun: o.DryRun, Version: o.Version, GitTag: o.GitTag, Revision: o.GitHead, Images: []pipeline.ImageResult{}})
}

func (f *syncFixture) testRecord(t *testing.T, r pipeline.TestRecord) {
	t.Helper()
	if _, err := pipeline.WriteTestRecord(f.top, r); err != nil {
		t.Fatal(err)
	}
}

func (f *syncFixture) docker(t *testing.T, d pipeline.DockerResult) {
	t.Helper()
	if _, err := pipeline.WriteDockerResult(f.top, d); err != nil {
		t.Fatal(err)
	}
}

func mainRelease() pipeline.ReleaseOutputs {
	return pipeline.ReleaseOutputs{Step: "release", Status: "passed", Published: true, Branch: "main", Version: "1.4.0", GitTag: "v1.4.0", GitHead: "c0ffee", BaseCommit: "base0"}
}

func (f *syncFixture) run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	opts := testOptions(t.TempDir())
	opts.Runner = &envtest.FakeRunner{Responses: map[string]envtest.Response{"git -C . rev-parse --show-toplevel": {Stdout: f.top + "\n"}}}
	opts.IsTerminal = func() bool { return f.terminal }
	opts.Stdin = strings.NewReader(f.stdin)
	opts.SignalContext = func(parent context.Context) (context.Context, context.CancelFunc, func() os.Signal) {
		ctx, cancel := context.WithCancel(parent)
		f.cancel = cancel
		return ctx, cancel, func() os.Signal { return f.signal }
	}
	opts.OpenSyncWorkspace = func(_ context.Context, o semrel.WorkspaceOptions) (syncWorkspace, error) {
		f.ws.opened = &o
		return f.ws, nil
	}
	var out, errOut bytes.Buffer
	code := run(append([]string{"pipeline", "sync"}, args...), &out, &errOut, opts)
	return code, out.String(), errOut.String()
}

func readSyncJSON(t *testing.T, top string) (pipeline.SyncResult, bool) {
	t.Helper()
	data, err := os.ReadFile(pipeline.SyncResultPath(top))
	if errors.Is(err, os.ErrNotExist) {
		return pipeline.SyncResult{}, false
	}
	var r pipeline.SyncResult
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	return r, true
}

func TestPipelineSync_MainMergesIntoDevelopAndEveryReleaseBranch(t *testing.T) {
	f := newSyncFixture(t, pipelineSyncYAML)
	f.release(t, mainRelease())
	code, stdout, stderr := f.run(t, "--yes", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	want := []string{
		"chore(sync): merge main into develop [skip ci]",
		"chore(sync): merge main into release/a [skip ci]",
		"chore(sync): merge main into release/b [skip ci]",
	}
	if !slices.Equal(f.ws.merges, want) || !slices.Equal(f.ws.pushes, []string{"develop", "release/a", "release/b"}) {
		t.Errorf("merges %q pushes %q", f.ws.merges, f.ws.pushes)
	}
	if f.ws.opened == nil || f.ws.opened.Branch != "main" || f.ws.pushCheck != 1 || !f.ws.closed {
		t.Errorf("workspace opened %+v push checks %d closed %v", f.ws.opened, f.ws.pushCheck, f.ws.closed)
	}
	var res pipeline.SyncResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("%v: %s", err, stdout)
	}
	if res.Status != "passed" || len(res.Branches) != 3 || res.Branches[0].Status != "pushed" || res.Branches[0].Commit == "" || res.Identity != "Dev <dev@example.com>" {
		t.Errorf("result %+v", res)
	}
	if saved, ok := readSyncJSON(t, f.top); !ok || saved.Status != "passed" || len(saved.Branches) != 3 {
		t.Errorf("sync.json %+v %v", saved, ok)
	}
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if !strings.HasPrefix(line, "[pipeline:sync] ") {
			t.Errorf("unprefixed line %q", line)
		}
	}
	if !strings.Contains(stderr, "Dev <dev@example.com>") {
		t.Errorf("the commit identity is not reported:\n%s", stderr)
	}
}

func TestPipelineSync_ReleaseBranchSyncsDevelopOnly(t *testing.T) {
	f := newSyncFixture(t, pipelineSyncYAML)
	o := mainRelease()
	o.Branch, o.Version = "release/a", "1.4.0-rc.1"
	f.release(t, o)
	if code, _, stderr := f.run(t, "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if !slices.Equal(f.ws.merges, []string{"chore(sync): merge release/a into develop [skip ci]"}) {
		t.Errorf("merges %q", f.ws.merges)
	}
}

func TestPipelineSync_NothingToSync(t *testing.T) {
	for _, tt := range []struct {
		name, yaml string
		release    func(*pipeline.ReleaseOutputs)
		want       string
	}{
		{"develop release", pipelineSyncYAML, func(o *pipeline.ReleaseOutputs) { o.Branch = "develop" }, "nothing to sync"},
		{"hotfix release", pipelineSyncYAML, func(o *pipeline.ReleaseOutputs) { o.Branch = "hotfix/x" }, "nothing to sync"},
		{"no new release", pipelineSyncYAML, func(o *pipeline.ReleaseOutputs) { o.Published = false }, "no new release"},
		{"disabled", pipelineSyncYAML + "  sync:\n    enabled: false\n", nil, "pipeline.sync.enabled is false"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newSyncFixture(t, tt.yaml)
			o := mainRelease()
			if tt.release != nil {
				tt.release(&o)
			}
			f.release(t, o)
			code, _, stderr := f.run(t)
			if code != 0 || !strings.Contains(stderr, tt.want) {
				t.Errorf("exit %d, stderr:\n%s", code, stderr)
			}
			if len(f.ws.merges) != 0 || len(f.ws.pushes) != 0 {
				t.Errorf("merges %q pushes %q", f.ws.merges, f.ws.pushes)
			}
			if r, ok := readSyncJSON(t, f.top); !ok || r.Status != "passed" || len(r.Branches) != 0 {
				t.Errorf("sync.json %+v %v", r, ok)
			}
		})
	}
}

func TestPipelineSync_ConflictIsReportedAndFailsAtTheEnd(t *testing.T) {
	f := newSyncFixture(t, pipelineSyncYAML)
	f.ws.conflicts = map[string]bool{"release/a": true}
	f.release(t, mainRelease())
	code, stdout, stderr := f.run(t, "--yes")
	if code != 2 {
		t.Fatalf("exit %d, want 2\n%s\n%s", code, stdout, stderr)
	}
	if !slices.Equal(f.ws.pushes, []string{"develop", "release/b"}) {
		t.Errorf("pushes %q: the branch after the conflict must still sync", f.ws.pushes)
	}
	r, ok := readSyncJSON(t, f.top)
	if !ok || r.Status != "failed" || r.Branches[1].Status != "conflict" || !strings.Contains(r.Error, "release/a") {
		t.Errorf("sync.json %+v %v", r, ok)
	}
	if !strings.Contains(stdout, "release/a <- main: conflict") || !strings.Contains(stderr, "merge-back failed for: release/a") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

func TestPipelineSync_DryRunPreviewsOnly(t *testing.T) {
	f := newSyncFixture(t, pipelineSyncYAML)
	f.ws.conflicts = map[string]bool{"release/b": true}
	f.release(t, mainRelease())
	code, stdout, stderr := f.run(t, "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if len(f.ws.merges) != 0 || len(f.ws.pushes) != 0 || f.ws.pushCheck != 0 || len(f.ws.previews) != 3 {
		t.Errorf("merges %q pushes %q push checks %d previews %q", f.ws.merges, f.ws.pushes, f.ws.pushCheck, f.ws.previews)
	}
	var res pipeline.SyncResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || res.Branches[2].Status != "would-conflict" || res.Branches[0].Status != "would-merge" {
		t.Errorf("result %+v", res)
	}
}

func TestPipelineSync_Refusals(t *testing.T) {
	rm := func(path func(string) string) func(*syncFixture) {
		return func(f *syncFixture) { os.Remove(path(f.top)) }
	}
	tests := []struct {
		name    string
		yaml    string
		release *pipeline.ReleaseOutputs
		args    []string
		setup   func(*syncFixture)
		code    int
		want    string
	}{
		{name: "no section", yaml: "enabled: true\n", code: 1, want: "no pipeline: section"},
		{name: "no release.json", args: []string{"--yes"}, code: 1, want: "run nerv pipeline release first"},
		{name: "failed release", release: &pipeline.ReleaseOutputs{Step: "release", Status: "failed"}, args: []string{"--yes"}, code: 1, want: "did not pass"},
		{name: "dry-run release", release: func() *pipeline.ReleaseOutputs { o := mainRelease(); o.DryRun = true; return &o }(), args: []string{"--yes"}, code: 1, want: "dry run"},
		{name: "release without base commit", release: func() *pipeline.ReleaseOutputs { o := mainRelease(); o.BaseCommit = ""; return &o }(), args: []string{"--yes"}, code: 1, want: "base_commit"},

		{name: "no test record", setup: rm(pipeline.TestRecordPath), code: 1, want: "test: no test record"},
		{name: "no test record in a dry run", args: []string{"--dry-run"}, setup: rm(pipeline.TestRecordPath), code: 1, want: "test: no test record"},
		{name: "failed test record", setup: func(f *syncFixture) {
			f.testRecord(t, pipeline.TestRecord{Step: "test", Status: "failed", Commit: "base0"})
		}, code: 1, want: "test: the test record did not pass"},
		{name: "tested uncommitted changes", setup: func(f *syncFixture) {
			f.testRecord(t, pipeline.TestRecord{Step: "test", Status: "passed", Commit: "base0", Dirty: true, Changes: []string{"src/App.cs"}})
		}, code: 1, want: "test: tested uncommitted changes (src/App.cs)"},
		{name: "stale test record", setup: func(f *syncFixture) {
			f.testRecord(t, pipeline.TestRecord{Step: "test", Status: "passed", Commit: "older0"})
		}, code: 1, want: "test: the test record is for commit older0, but the release was made from base0"},

		{name: "no docker.json", setup: rm(pipeline.DockerResultPath), code: 1, want: "docker: no docker.json"},
		{name: "failed docker", setup: func(f *syncFixture) {
			f.docker(t, pipeline.DockerResult{Step: "docker", Status: "failed", Version: "1.4.0", GitTag: "v1.4.0", Revision: "c0ffee"})
		}, code: 1, want: "docker: docker.json did not pass"},
		{name: "docker.json of another version", setup: func(f *syncFixture) {
			f.docker(t, pipeline.DockerResult{Step: "docker", Status: "passed", Version: "1.3.0", GitTag: "v1.3.0", Revision: "0ld"})
		}, code: 1, want: "docker: docker.json is for 1.3.0 (v1.3.0, 0ld), not this release 1.4.0 (v1.4.0, c0ffee)"},
		{name: "docker.json of another commit", setup: func(f *syncFixture) {
			f.docker(t, pipeline.DockerResult{Step: "docker", Status: "passed", Version: "1.4.0", GitTag: "v1.4.0", Revision: "other"})
		}, code: 1, want: "docker: docker.json is for 1.4.0 (v1.4.0, other)"},
		{name: "dry-run docker.json for a real release", setup: func(f *syncFixture) {
			f.docker(t, pipeline.DockerResult{Step: "docker", Status: "passed", DryRun: true, Version: "1.4.0", GitTag: "v1.4.0", Revision: "c0ffee"})
		}, code: 1, want: "docker: docker.json comes from a dry run"},

		{name: "no terminal", code: 1, want: "pass --yes"},
		{name: "declined", setup: func(f *syncFixture) { f.terminal, f.stdin = true, "n\n" }, code: 1, want: "cancelled"},
		{name: "no commit identity", setup: func(f *syncFixture) {
			f.ws.identErr = fmt.Errorf("%w for /repo: set user.name and user.email", semrel.ErrNoCommitIdentity)
		}, code: 1, want: "no git commit identity is configured"},
		{name: "push denied", args: []string{"--yes"}, setup: func(f *syncFixture) { f.ws.pushErr = semrel.ErrPushDenied }, code: 2, want: "cannot push"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := tt.yaml
			if yaml == "" {
				yaml = pipelineSyncYAML
			}
			f := newSyncFixture(t, yaml)
			switch {
			case tt.release != nil:
				f.release(t, *tt.release)
			case tt.name != "no section" && tt.name != "no release.json":
				f.release(t, mainRelease())
			}
			if tt.setup != nil {
				tt.setup(f)
			}
			args := tt.args
			if args == nil {
				args = []string{"--yes"}
				if tt.name == "no terminal" || tt.name == "declined" {
					args = nil
				}
			}
			code, _, stderr := f.run(t, args...)
			if code != tt.code || !strings.Contains(stderr, tt.want) {
				t.Errorf("exit %d (want %d), stderr:\n%s", code, tt.code, stderr)
			}
			if len(f.ws.merges) != 0 || len(f.ws.pushes) != 0 {
				t.Errorf("merges %q pushes %q", f.ws.merges, f.ws.pushes)
			}
			if evidence := strings.HasPrefix(tt.want, "test:") || strings.HasPrefix(tt.want, "docker:"); evidence && f.ws.opened != nil {
				t.Error("the clone was opened before the evidence was checked")
			}
		})
	}
}

func TestPipelineSync_DryRunReleaseChainPreviews(t *testing.T) {
	f := newSyncFixture(t, pipelineSyncYAML)
	o := mainRelease()
	o.DryRun, o.GitHead = true, "base0"
	f.release(t, o)
	if code, _, stderr := f.run(t, "--dry-run"); code != 0 || len(f.ws.previews) != 3 {
		t.Errorf("exit %d previews %q\n%s", code, f.ws.previews, stderr)
	}
}

func TestPipelineSync_ConfirmedOnATerminal(t *testing.T) {
	f := newSyncFixture(t, pipelineSyncYAML)
	f.terminal, f.stdin = true, "y\n"
	f.release(t, mainRelease())
	code, _, stderr := f.run(t)
	if code != 0 || len(f.ws.pushes) != 3 {
		t.Fatalf("exit %d pushes %q\n%s", code, f.ws.pushes, stderr)
	}
	for _, want := range []string{"merge main into develop and push", "merge main into release/b and push", "[y/N]"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
}

func TestPipelineSync_InterruptedAfterAPushRecordsWhatFinished(t *testing.T) {
	f := newSyncFixture(t, pipelineSyncYAML)
	f.release(t, mainRelease())
	f.signal = syscall.SIGINT
	f.ws.onPush = func() { f.cancel() }
	code, stdout, stderr := f.run(t, "--yes", "--json")
	if code != 130 {
		t.Fatalf("exit %d, want 130\n%s\n%s", code, stdout, stderr)
	}
	if !slices.Equal(f.ws.pushes, []string{"develop"}) {
		t.Errorf("pushes %q: nothing may run after the interruption", f.ws.pushes)
	}
	r, ok := readSyncJSON(t, f.top)
	if !ok {
		t.Fatalf("no sync.json after an interrupted run that pushed develop\n%s", stderr)
	}
	want := []string{"develop:pushed", "release/a:not-run", "release/b:not-run"}
	var got []string
	for _, b := range r.Branches {
		got = append(got, b.Branch+":"+b.Status)
	}
	if r.Status != "interrupted" || !slices.Equal(got, want) || r.Branches[0].Commit == "" {
		t.Errorf("sync.json status %s branches %q (%+v), want interrupted %q", r.Status, got, r.Branches, want)
	}
	var doc pipeline.SyncResult
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil || doc.Status != "interrupted" || len(doc.Branches) != 3 {
		t.Errorf("stdout %s %v", stdout, err)
	}
}

func ptr[T any](v T) *T { return &v }

func TestPipelineSync_NoCloneWhenTheReleasedBranchSyncsNothing(t *testing.T) {
	for _, branch := range []string{"develop", "hotfix/x"} {
		f := newSyncFixture(t, pipelineSyncYAML)
		o := mainRelease()
		o.Branch = branch
		f.release(t, o)
		code, _, stderr := f.run(t, "--yes")
		if code != 0 || f.ws.opened != nil || !strings.Contains(stderr, "nothing to sync") {
			t.Errorf("%s: exit %d, clone opened %v\n%s", branch, code, f.ws.opened != nil, stderr)
		}
	}
}
