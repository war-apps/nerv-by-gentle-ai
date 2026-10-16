package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

const pipelineDockerYAML = `pipeline:
  dotnet:
    solution: App.slnx
  registry:
    host: ghcr.io
  images:
    - name: ghcr.io/Acme/API
      title: acme.api
      build_args:
        APP_VERSION: "${version}"
`

type fakeDockerWorkspace struct {
	dir, head, public string
	tags              map[string]string
	opened            *semrel.WorkspaceOptions
	checkedOut        []string
	closed            bool
}

func (w *fakeDockerWorkspace) Dir() string        { return w.dir }
func (w *fakeDockerWorkspace) PublicURL() string  { return w.public }
func (w *fakeDockerWorkspace) OriginHead() string { return w.head }
func (w *fakeDockerWorkspace) Close() error       { w.closed = true; return nil }
func (w *fakeDockerWorkspace) CheckoutTag(_ context.Context, tag string) (string, error) {
	w.checkedOut = append(w.checkedOut, tag)
	if c, ok := w.tags[tag]; ok {
		return c, nil
	}
	return "", semrel.ErrTagNotOnOrigin
}

type recProc struct {
	cmds []proc.Command
	fail func(proc.Command) error
}

func (p *recProc) Run(_ context.Context, c proc.Command) (proc.Result, error) {
	p.cmds = append(p.cmds, c)
	if p.fail != nil {
		return proc.Result{}, p.fail(c)
	}
	return proc.Result{}, nil
}

func (p *recProc) sub(name string) [][]string {
	var out [][]string
	for _, c := range p.cmds {
		if c.Args[0] == name {
			out = append(out, c.Args)
		}
	}
	return out
}

type noArgs struct{}

func (noArgs) Capture(context.Context, shiprun.Step, shiprun.Vars) (string, error) {
	return "", errors.New("no computed args expected")
}

type dockerFixture struct {
	top  string
	ws   *fakeDockerWorkspace
	proc *recProc
	env  map[string]string
}

func newDockerFixture(t *testing.T, yaml string) *dockerFixture {
	t.Helper()
	top := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(top, ".nerv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, ".nerv", "nerv.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return &dockerFixture{top: top, ws: &fakeDockerWorkspace{dir: t.TempDir(), head: "branchhead", public: "https://github.com/acme/api.git", tags: map[string]string{"v1.4.0-alpha.3": "c0ffee"}},
		proc: &recProc{}, env: map[string]string{"GHCR_USER": "bot", "GHCR_TOKEN": "s3cr3t"}}
}

func (f *dockerFixture) release(t *testing.T, o pipeline.ReleaseOutputs) {
	t.Helper()
	if _, err := pipeline.WriteReleaseOutputs(f.top, o); err != nil {
		t.Fatal(err)
	}
}

func alphaRelease() pipeline.ReleaseOutputs {
	return pipeline.ReleaseOutputs{Step: "release", Status: "passed", Published: true, Branch: "develop", Version: "1.4.0-alpha.3", Channel: "alpha", GitTag: "v1.4.0-alpha.3", GitHead: "c0ffee"}
}

func (f *dockerFixture) run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	opts := testOptions(t.TempDir())
	opts.Runner = &envtest.FakeRunner{Responses: map[string]envtest.Response{"git -C . rev-parse --show-toplevel": {Stdout: f.top + "\n"}}}
	opts.Getenv = func(k string) string { return f.env[k] }
	opts.OpenDockerWorkspace = func(_ context.Context, o semrel.WorkspaceOptions) (dockerWorkspace, error) {
		f.ws.opened = &o
		return f.ws, nil
	}
	opts.DockerProc = f.proc
	opts.DockerArgs = func(string, io.Writer) pipeline.ArgRunner { return noArgs{} }
	var out, errOut bytes.Buffer
	code := run(append([]string{"pipeline", "docker"}, args...), &out, &errOut, opts)
	return code, out.String(), errOut.String()
}

func readDockerJSON(t *testing.T, top string) (pipeline.DockerResult, bool) {
	t.Helper()
	data, err := os.ReadFile(pipeline.DockerResultPath(top))
	if errors.Is(err, os.ErrNotExist) {
		return pipeline.DockerResult{}, false
	}
	var r pipeline.DockerResult
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	return r, true
}

func TestPipelineDocker_BuildsTheReleasedCommitAndPushes(t *testing.T) {
	f := newDockerFixture(t, pipelineDockerYAML)
	f.release(t, alphaRelease())
	code, stdout, stderr := f.run(t, "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if f.ws.opened.Branch != "develop" || f.ws.opened.FetchTimeout != semrel.DefaultFetchTimeout || !slices.Equal(f.ws.checkedOut, []string{"v1.4.0-alpha.3"}) || !f.ws.closed {
		t.Errorf("workspace %+v, checked out %v", f.ws.opened, f.ws.checkedOut)
	}
	login := f.proc.cmds[0]
	if !slices.Equal(login.Args, []string{"login", "ghcr.io", "-u", "bot", "--password-stdin"}) || string(login.Stdin) != "s3cr3t\n" {
		t.Errorf("login %q", login.Args)
	}
	build := f.proc.sub("build")[0]
	for _, want := range []string{"APP_VERSION=1.4.0-alpha.3", "org.opencontainers.image.revision=c0ffee", "org.opencontainers.image.source=https://github.com/acme/api", "ghcr.io/acme/api:1.4.0-alpha.3", "ghcr.io/acme/api:alpha"} {
		if !slices.Contains(build, want) {
			t.Errorf("build %q lacks %q", build, want)
		}
	}
	if len(f.proc.sub("push")) != 2 {
		t.Errorf("pushes %q", f.proc.sub("push"))
	}
	r, ok := readDockerJSON(t, f.top)
	if !ok || r.Status != "passed" || r.Version != "1.4.0-alpha.3" || len(r.Images) != 1 || r.Images[0].Status != "pushed" {
		t.Errorf("docker.json %+v", r)
	}
	var doc pipeline.DockerResult
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil || doc.Images[0].Name != "ghcr.io/acme/api" {
		t.Errorf("stdout %q %v", stdout, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if !strings.HasPrefix(line, "[pipeline:docker] ") {
			t.Errorf("unprefixed line %q", line)
		}
	}
	if strings.Contains(stdout+stderr, "s3cr3t") {
		t.Error("token printed")
	}
}

func TestPipelineDocker_NoNewReleaseBuildsNothing(t *testing.T) {
	f := newDockerFixture(t, pipelineDockerYAML)
	f.release(t, pipeline.ReleaseOutputs{Step: "release", Status: "passed", Published: false, Branch: "develop"})
	code, stdout, _ := f.run(t)
	r, ok := readDockerJSON(t, f.top)
	if code != 0 || len(f.proc.cmds) != 0 || f.ws.opened != nil || !ok || len(r.Images) != 0 || !strings.Contains(stdout, "no new release") {
		t.Errorf("exit %d, %d commands, docker.json %+v, stdout %q", code, len(f.proc.cmds), r, stdout)
	}
}

func TestPipelineDocker_DryRunRelease(t *testing.T) {
	f := newDockerFixture(t, pipelineDockerYAML)
	rel := alphaRelease()
	rel.DryRun = true
	f.release(t, rel)
	if code, _, stderr := f.run(t); code != 1 || !strings.Contains(stderr, "dry run") || len(f.proc.cmds) != 0 {
		t.Errorf("real docker on a dry-run release: exit %d\n%s", code, stderr)
	}
	code, _, stderr := f.run(t, "--dry-run")
	if code != 0 || len(f.ws.checkedOut) != 0 || len(f.proc.sub("login")) != 0 || len(f.proc.sub("push")) != 0 || len(f.proc.sub("build")) != 1 {
		t.Fatalf("exit %d, commands %v\n%s", code, f.proc.cmds, stderr)
	}
	if !slices.Contains(f.proc.sub("build")[0], "org.opencontainers.image.revision=branchhead") {
		t.Errorf("dry run builds origin's branch head: %q", f.proc.sub("build")[0])
	}
}

func TestPipelineDocker_Refusals(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *dockerFixture)
		args  []string
		want  string
	}{
		{"no release.json", func(f *dockerFixture) {}, nil, "nerv pipeline release"},
		{"failed release", func(f *dockerFixture) {
			r := alphaRelease()
			r.Status = "failed"
			f.release(t, r)
		}, nil, "nerv pipeline release"},
		{"tag not on origin", func(f *dockerFixture) {
			f.release(t, alphaRelease())
			f.ws.tags = map[string]string{}
		}, nil, "v1.4.0-alpha.3"},
		{"tag moved", func(f *dockerFixture) {
			f.release(t, alphaRelease())
			f.ws.tags["v1.4.0-alpha.3"] = "other"
		}, nil, "other"},
		{"no credentials", func(f *dockerFixture) {
			f.release(t, alphaRelease())
			f.env = map[string]string{}
		}, nil, "GHCR_TOKEN"},
		{"no pipeline section", func(f *dockerFixture) {
			os.WriteFile(filepath.Join(f.top, ".nerv", "nerv.yaml"), []byte("enabled: true\n"), 0o644)
		}, nil, "docs/pipeline.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDockerFixture(t, pipelineDockerYAML)
			tt.setup(f)
			code, stdout, _ := f.run(t, append([]string{"--json"}, tt.args...)...)
			var doc pipeline.DockerResult
			_ = json.Unmarshal([]byte(stdout), &doc)
			if code != 1 || doc.Status != "refused" || !strings.Contains(doc.Error, tt.want) || len(f.proc.sub("build")) != 0 {
				t.Errorf("exit %d, %+v", code, doc)
			}
		})
	}
}

func TestPipelineDocker_NoLoginUsesDockersOwnCredentials(t *testing.T) {
	f := newDockerFixture(t, pipelineDockerYAML)
	f.release(t, alphaRelease())
	f.env = map[string]string{}
	if code, _, stderr := f.run(t, "--no-login"); code != 0 || len(f.proc.sub("login")) != 0 || len(f.proc.sub("push")) != 2 {
		t.Errorf("exit %d\n%s", code, stderr)
	}
}

func TestPipelineDocker_FailureLeavesNoDockerJSON(t *testing.T) {
	f := newDockerFixture(t, pipelineDockerYAML)
	f.release(t, alphaRelease())
	if _, err := pipeline.WriteDockerResult(f.top, pipeline.DockerResult{Step: "docker", Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	f.proc.fail = func(c proc.Command) error {
		if c.Args[0] == "build" {
			return errors.New("exit status 1")
		}
		return nil
	}
	code, stdout, _ := f.run(t, "--json")
	var doc pipeline.DockerResult
	_ = json.Unmarshal([]byte(stdout), &doc)
	if code != 2 || doc.Status != "failed" || doc.Images[0].Status != "failed" {
		t.Errorf("exit %d, %+v", code, doc)
	}
	if _, ok := readDockerJSON(t, f.top); ok {
		t.Error("a stale docker.json survived a failed run")
	}
}
