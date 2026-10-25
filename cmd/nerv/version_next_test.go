package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseclone"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseplan"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

// releaseconfigValidationError is a release configuration problem as the engine
// reports it.
var releaseconfigValidationError = releaseconfig.ValidationError{Problems: []string{"release.projects[0].path: required"}}

// nextFixture is a fake repository for `nerv version next`: git's top-level
// answer comes from a scripted runner, the release configuration (if any)
// from a real .ai-workflow/config.yaml in a temp dir, and the engine is a recorded
// fake.
type nextFixture struct {
	top     string
	runner  *envtest.FakeRunner
	calls   []releaseclone.ComputeOptions
	ctxErrs []error
	results []versioning.Result
	err     error
	// stopped counts the cancel functions the signal seam handed out that
	// were called.
	stopped int
	// signalCtx, when set, replaces the context the signal seam returns.
	signalCtx context.Context
	// signal is what the signal seam reports as received.
	signal os.Signal
	// stdout, when set, replaces the buffer the command writes its result to.
	stdout io.Writer
}

func newNextFixture(t *testing.T, nervYAML string) *nextFixture {
	t.Helper()
	// The repository directory is named "repo": without a release: section
	// the engine names the only project after it.
	top := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(top, 0o755); err != nil {
		t.Fatal(err)
	}
	if nervYAML != "" {
		if err := os.MkdirAll(filepath.Join(top, ".nerv"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(top, ".ai-workflow"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(top, ".ai-workflow/config.yaml"), []byte(nervYAML), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &nextFixture{
		top: top,
		runner: &envtest.FakeRunner{Responses: map[string]envtest.Response{
			"git -C . rev-parse --show-toplevel": {Stdout: top + "\n"},
		}},
	}
}

func (f *nextFixture) options(t *testing.T) options {
	opts := testOptions(t.TempDir())
	opts.Runner = f.runner
	opts.ComputeVersions = func(ctx context.Context, o releaseclone.ComputeOptions) ([]versioning.Result, error) {
		f.calls = append(f.calls, o)
		f.ctxErrs = append(f.ctxErrs, ctx.Err())
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return f.results, f.err
	}
	opts.SignalContext = func(parent context.Context) (context.Context, context.CancelFunc, func() os.Signal) {
		ctx := parent
		if f.signalCtx != nil {
			ctx = f.signalCtx
		}
		ctx, cancel := context.WithCancel(ctx)
		return ctx, func() { f.stopped++; cancel() }, func() os.Signal { return f.signal }
	}
	return opts
}

func (f *nextFixture) run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	var w io.Writer = &out
	if f.stdout != nil {
		w = f.stdout
	}
	code = run(append([]string{"version", "next"}, args...), w, &errOut, f.options(t))
	return code, out.String(), errOut.String()
}

const twoProjectsYAML = `enabled: true
release:
  projects:
    - name: api
      path: api
    - name: web-app
      path: web
`

func released(project, version, channel string) versioning.Result {
	parts := strings.SplitN(strings.SplitN(version, "-", 2)[0], ".", 3)
	return versioning.Result{
		Project: project, Branch: "develop",
		NewReleasePublished:    true,
		NewReleaseVersion:      version,
		NewReleaseMajorVersion: parts[0], NewReleaseMinorVersion: parts[1], NewReleasePatchVersion: parts[2],
		NewReleaseChannel:  channel,
		NewReleaseNotes:    "## " + version + "\n\n### Features\n\n* thing\n",
		NewReleaseGitHead:  "abc123",
		NewReleaseGitTag:   "v" + version,
		LastReleaseVersion: "1.1.0", LastReleaseGitHead: "def456", LastReleaseGitTag: "v1.1.0",
	}
}

func notReleased(project string) versioning.Result {
	return versioning.Result{
		Project: project, Branch: "main",
		LastReleaseVersion: "1.1.0", LastReleaseGitHead: "def456", LastReleaseGitTag: "v1.1.0",
	}
}

// ---------------------------------------------------------------------------
// Dispatch: bare `version` is unchanged, `version next` computes, anything
// else after `version` is a usage error.
// ---------------------------------------------------------------------------

func TestRun_VersionUnknownSubcommandIsAUsageError(t *testing.T) {
	for _, args := range [][]string{{"version", "bogus"}, {"version", "--json", "extra"}, {"version", "--offline"}} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr, testOptions(t.TempDir()))
		if code != 2 {
			t.Errorf("%v: exit code = %d, want 2", args, code)
		}
		if stdout.Len() != 0 || !strings.Contains(stderr.String(), "version next") {
			t.Errorf("%v: stdout=%q stderr=%q, want usage (naming `version next`) on stderr only", args, stdout.String(), stderr.String())
		}
	}
}

func TestVersionNext_Help(t *testing.T) {
	f := newNextFixture(t, "")
	code, stdout, _ := f.run(t, "--help")
	if code != 0 || !strings.Contains(stdout, "Usage: nerv version next") || !strings.Contains(stdout, "--offline") {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
	if len(f.calls) != 0 {
		t.Error("--help computed versions")
	}
}

func TestVersionNext_UsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown flag", []string{"--bogus"}},
		{"positional argument", []string{"develop"}},
		{"unknown format", []string{"--format", "yaml"}},
		{"json conflicts with env", []string{"--json", "--format", "env"}},
		{"negative fetch timeout", []string{"--fetch-timeout", "-1s"}},
		{"zero fetch timeout", []string{"--fetch-timeout", "0"}},
		{"zero fetch timeout with a unit", []string{"--fetch-timeout", "0s"}},
		{"malformed fetch timeout", []string{"--fetch-timeout", "soon"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newNextFixture(t, "")
			code, stdout, stderr := f.run(t, tt.args...)
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout != "" || !strings.Contains(stderr, "Usage: nerv version next") {
				t.Errorf("stdout=%q stderr=%q", stdout, stderr)
			}
			if len(f.calls) != 0 {
				t.Error("a usage error computed versions")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Flags reach the engine.
// ---------------------------------------------------------------------------

func TestVersionNext_FlagsReachTheEngine(t *testing.T) {
	f := newNextFixture(t, twoProjectsYAML)
	f.runner.Responses["git -C /work/repo/sub rev-parse --show-toplevel"] = envtest.Response{Stdout: f.top + "\n"}
	f.results = []versioning.Result{released("api", "1.2.0", "")}

	code, _, stderr := f.run(t, "--cwd", "/work/repo/sub", "--branch", "develop", "--project", "api", "--offline", "--fetch-timeout", "15s")

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	want := releaseclone.ComputeOptions{RepoRoot: f.top, Branch: "develop", Projects: []string{"api"}, Offline: true, FetchTimeout: 15 * time.Second}
	if len(f.calls) != 1 {
		t.Fatalf("engine calls = %+v", f.calls)
	}
	got := f.calls[0]
	engine := got.Engine
	got.Engine = nil
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("engine options = %+v, want %+v", got, want)
	}
	// The release: section's projects reach its version engine, the
	// default conventional-commits.
	if engine == nil || engine.Name() != "conventional-commits" || len(engine.Projects()) != 2 || engine.Projects()[0].Name != "api" || engine.Projects()[1].Path != "web" {
		t.Errorf("engine = %+v", engine)
	}
}

func TestVersionNext_DefaultsReadOriginOnTheCurrentBranch(t *testing.T) {
	f := newNextFixture(t, "")
	f.results = []versioning.Result{notReleased("repo")}

	code, stdout, stderr := f.run(t)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	want := releaseclone.ComputeOptions{RepoRoot: f.top, FetchTimeout: releaseclone.DefaultFetchTimeout}
	var engine *releaseplan.VersionEngine
	if len(f.calls) == 1 {
		engine, f.calls[0].Engine = f.calls[0].Engine, nil
	}
	if engine == nil || engine.Name() != "conventional-commits" || len(engine.Projects()) != 0 {
		t.Errorf("engine = %+v, want the default engine for the whole repository", engine)
	}
	if len(f.calls) != 1 || !reflect.DeepEqual(f.calls[0], want) {
		t.Fatalf("engine options = %+v, want %+v (current branch, origin read, default timeout, the default configuration)", f.calls, want)
	}
	if !strings.Contains(stderr, "origin") {
		t.Errorf("stderr = %q, want a progress line saying origin is read", stderr)
	}
	if strings.Contains(stdout, "origin") {
		t.Errorf("progress leaked to stdout: %q", stdout)
	}
}

// ---------------------------------------------------------------------------
// Formats.
// ---------------------------------------------------------------------------

func TestVersionNext_JSONSingleProjectIsAnObject(t *testing.T) {
	for _, args := range [][]string{{"--json"}, {"--format", "json"}, {"--json", "--format", "json"}} {
		f := newNextFixture(t, "")
		f.results = []versioning.Result{released("repo", "1.2.0-alpha.2", "alpha")}

		code, stdout, stderr := f.run(t, args...)

		if code != 0 {
			t.Fatalf("%v: exit code = %d, stderr = %q", args, code, stderr)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("%v: stdout is not one JSON object: %v (%q)", args, err, stdout)
		}
		if _, ok := got["semantic_release_config"]; len(got) != 15 || !ok {
			t.Errorf("%v: %d keys, want project, branch, the 12 outputs and semantic_release_config: %v", args, len(got), got)
		}
		if got["new_release_version"] != "1.2.0-alpha.2" || got["new_release_channel"] != "alpha" || got["new_release_published"] != true || got["project"] != "repo" || got["branch"] != "develop" {
			t.Errorf("%v: got %v", args, got)
		}
	}
}

func TestVersionNext_JSONSeveralProjectsIsAnArray(t *testing.T) {
	f := newNextFixture(t, twoProjectsYAML)
	f.results = []versioning.Result{released("api", "1.2.0", ""), notReleased("web-app")}

	code, stdout, _ := f.run(t, "--json")

	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	var got []versioning.Result
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not a JSON array: %v (%q)", err, stdout)
	}
	if len(got) != 2 || !reflect.DeepEqual(got[0], f.results[0]) || !reflect.DeepEqual(got[1], f.results[1]) {
		t.Errorf("got %+v", got)
	}
}

// parseEnv reads GitHub Actions output syntax: key=value lines and
// key<<DELIM ... DELIM blocks. It returns the values and the delimiters.
func parseEnv(t *testing.T, out string) (map[string]string, map[string]string) {
	t.Helper()
	values, delims := map[string]string{}, map[string]string{}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if k, d, ok := strings.Cut(line, "<<"); ok && !strings.Contains(k, "=") {
			var body []string
			i++
			for ; i < len(lines) && lines[i] != d; i++ {
				body = append(body, lines[i])
			}
			if i == len(lines) {
				t.Fatalf("heredoc %s<<%s is not closed:\n%s", k, d, out)
			}
			values[k], delims[k] = strings.Join(body, "\n"), d
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("line %q is neither key=value nor key<<DELIM", line)
		}
		values[k] = v
	}
	return values, delims
}

func TestVersionNext_EnvSingleProject(t *testing.T) {
	f := newNextFixture(t, "")
	r := released("repo", "1.2.0", "")
	f.results = []versioning.Result{r}

	code, stdout, _ := f.run(t, "--format", "env")

	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	values, delims := parseEnv(t, stdout)
	if len(values) != 12 {
		t.Errorf("%d keys, want the 12 outputs unprefixed: %v", len(values), values)
	}
	for _, o := range r.Outputs() {
		if values[o.Name] != o.Value {
			t.Errorf("%s = %q, want %q", o.Name, values[o.Name], o.Value)
		}
	}
	d, ok := delims["new_release_notes"]
	if !ok {
		t.Fatalf("new_release_notes is not a heredoc:\n%s", stdout)
	}
	if len(d) < 16 || strings.Contains(r.NewReleaseNotes, d) {
		t.Errorf("delimiter %q is short or occurs in the notes", d)
	}
	if !strings.Contains(stdout, "new_release_published=true\n") || !strings.Contains(stdout, "new_release_channel=\n") {
		t.Errorf("stdout:\n%s", stdout)
	}
}

func TestVersionNext_EnvNotesKeepTheirExactText(t *testing.T) {
	f := newNextFixture(t, "")
	r := released("repo", "1.2.0", "")
	r.NewReleaseNotes = "line one\n\nline three\n"
	f.results = []versioning.Result{r}

	_, stdout, _ := f.run(t, "--format", "env")

	_, delims := parseEnv(t, stdout)
	d := delims["new_release_notes"]
	want := "new_release_notes<<" + d + "\nline one\n\nline three\n\n" + d + "\n"
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout:\n%s\nwant block:\n%s", stdout, want)
	}
}

func TestVersionNext_EnvDelimitersAreUnique(t *testing.T) {
	f := newNextFixture(t, twoProjectsYAML)
	f.results = []versioning.Result{released("api", "1.2.0", ""), released("web-app", "0.3.1", "")}

	_, stdout, _ := f.run(t, "--format", "env")

	_, delims := parseEnv(t, stdout)
	if delims["api_new_release_notes"] == delims["web_app_new_release_notes"] {
		t.Errorf("both heredocs use delimiter %q", delims["api_new_release_notes"])
	}
}

func TestVersionNext_EnvSeveralProjectsArePrefixed(t *testing.T) {
	f := newNextFixture(t, twoProjectsYAML)
	f.results = []versioning.Result{released("api", "1.2.0", ""), notReleased("web-app")}

	code, stdout, _ := f.run(t, "--format", "env")

	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	values, _ := parseEnv(t, stdout)
	if len(values) != 24 {
		t.Errorf("%d keys, want 24", len(values))
	}
	for k, want := range map[string]string{
		"api_new_release_version":       "1.2.0",
		"api_new_release_published":     "true",
		"web_app_new_release_published": "false",
		"web_app_last_release_git_tag":  "v1.1.0",
	} {
		if values[k] != want {
			t.Errorf("%s = %q, want %q", k, values[k], want)
		}
	}
	if _, ok := values["new_release_version"]; ok {
		t.Error("unprefixed key with several projects")
	}
}

func TestVersionNext_Text(t *testing.T) {
	f := newNextFixture(t, twoProjectsYAML)
	f.results = []versioning.Result{released("api", "1.2.0-alpha.2", "alpha"), notReleased("web-app")}

	code, stdout, _ := f.run(t)

	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	for _, want := range []string{
		"api (develop)", "1.2.0-alpha.2", "channel alpha", "tag v1.2.0-alpha.2",
		"web-app (main)", "no new release",
		"last release: 1.1.0 (tag v1.1.0)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("text output lacks %q:\n%s", want, stdout)
		}
	}
}

func TestVersionNext_TextFirstReleaseAndDefaultChannel(t *testing.T) {
	f := newNextFixture(t, "")
	r := released("repo", "1.0.0", "")
	r.LastReleaseVersion, r.LastReleaseGitHead, r.LastReleaseGitTag = "", "", ""
	f.results = []versioning.Result{r}

	_, stdout, _ := f.run(t, "--format", "text")

	for _, want := range []string{"1.0.0", "default channel", "last release: none"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("text output lacks %q:\n%s", want, stdout)
		}
	}
}

func TestVersionNext_NoNewReleaseIsNotAnError(t *testing.T) {
	f := newNextFixture(t, "")
	f.results = []versioning.Result{notReleased("repo")}

	code, stdout, _ := f.run(t, "--format", "env")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "new_release_published=false\n") {
		t.Errorf("stdout:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// Refusals (exit 1) and environment errors (exit 2).
// ---------------------------------------------------------------------------

func TestVersionNext_UnknownProjectListsTheProjects(t *testing.T) {
	f := newNextFixture(t, twoProjectsYAML)

	code, stdout, stderr := f.run(t, "--project", "mobile")

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout != "" || !strings.Contains(stderr, `"mobile"`) || !strings.Contains(stderr, "api, web-app") {
		t.Errorf("stdout=%q stderr=%q", stdout, stderr)
	}
	if len(f.calls) != 0 {
		t.Error("an unknown project computed versions")
	}
}

func TestVersionNext_ProjectWithoutAReleaseSection(t *testing.T) {
	f := newNextFixture(t, "")

	code, _, stderr := f.run(t, "--project", "api")

	if code != 1 || !strings.Contains(stderr, "release:") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestVersionNext_InvalidConfigIsARefusal(t *testing.T) {
	f := newNextFixture(t, "release:\n  projects:\n    - name: API\n      path: api\n      colour: red\n")

	code, stdout, stderr := f.run(t)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout != "" || !strings.Contains(stderr, "release.projects[0].colour") || !strings.Contains(stderr, "release.projects[0].name") {
		t.Errorf("stdout=%q stderr=%q, want the path-qualified problems", stdout, stderr)
	}
	if len(f.calls) != 0 {
		t.Error("an invalid configuration computed versions")
	}
}

func TestVersionNext_NotAGitRepository(t *testing.T) {
	f := newNextFixture(t, "")
	f.runner.Responses["git -C . rev-parse --show-toplevel"] = envtest.Response{Stderr: "fatal: not a git repository", ExitCode: 128}

	code, _, stderr := f.run(t)

	if code != 1 || !strings.Contains(stderr, "not inside a git repository") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestVersionNext_GitMissing(t *testing.T) {
	f := newNextFixture(t, "")
	f.runner.Responses["git -C . rev-parse --show-toplevel"] = envtest.Response{ExitCode: -1, Err: &exec.Error{Name: "git", Err: exec.ErrNotFound}}

	code, _, stderr := f.run(t)

	if code != 2 || !strings.Contains(stderr, "git") || !strings.Contains(stderr, "PATH") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestVersionNext_EngineErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     int
		contains []string
		absent   []string
	}{
		{
			name: "git failure", code: 2,
			err:      errors.New("git log: fatal: bad object"),
			contains: []string{"git log: fatal: bad object"},
			absent:   []string{"--offline"},
		},
		{
			name: "git too old", code: 2,
			err:      errors.New("git 2.30.0 is too old: selecting a project's commits by path needs git 2.31.0 or newer"),
			contains: []string{"git 2.30.0 is too old"},
		},
		{
			name: "origin unreachable", code: 2,
			err:      fmt.Errorf("reading release tags and notes from origin (https://***@github.com/x/y.git): %w: exit status 128", releaseclone.ErrOriginFetch),
			contains: []string{"origin", "--offline"},
		},
		{
			name: "validation error from the engine", code: 1,
			err:      fmt.Errorf("wrapped: %w", &releaseconfigValidationError),
			contains: []string{"release.projects[0].path"},
		},
		{
			name: "engine configuration error", code: 1,
			err:      fmt.Errorf("wrapped: %w", &versioning.ConfigError{Problems: []string{"tag_format: bad"}}),
			contains: []string{"tag_format: bad"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newNextFixture(t, "")
			f.err = tt.err

			code, stdout, stderr := f.run(t)

			if code != tt.code {
				t.Errorf("exit code = %d, want %d", code, tt.code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			for _, want := range tt.contains {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr lacks %q: %q", want, stderr)
				}
			}
			for _, unwanted := range tt.absent {
				if strings.Contains(stderr, unwanted) {
					t.Errorf("stderr has %q: %q", unwanted, stderr)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Signals: the command runs under the signal context and releases it.
// ---------------------------------------------------------------------------

func TestVersionNext_RunsUnderTheSignalContext(t *testing.T) {
	f := newNextFixture(t, "")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel() // as if SIGINT arrived
	f.signalCtx = cancelled
	f.signal = syscall.SIGINT

	code, stdout, stderr := f.run(t)

	// The engine's validate already runs under the cancelled context, so
	// the computation, when it is reached at all, sees it cancelled too.
	if len(f.ctxErrs) > 1 || (len(f.ctxErrs) == 1 && f.ctxErrs[0] == nil) {
		t.Fatalf("the engine did not run under the signal context: %v", f.ctxErrs)
	}
	if code != 130 || stdout != "" || !strings.Contains(stderr, "interrupted") {
		t.Errorf("code=%d stdout=%q stderr=%q, want 130 (128+SIGINT)", code, stdout, stderr)
	}
	if f.stopped != 1 {
		t.Errorf("signal context released %d times, want 1", f.stopped)
	}
}

func TestVersionNext_ReleasesTheSignalContextOnSuccess(t *testing.T) {
	f := newNextFixture(t, "")
	f.results = []versioning.Result{notReleased("repo")}

	if code, _, _ := f.run(t); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if f.stopped != 1 {
		t.Errorf("signal context released %d times, want 1", f.stopped)
	}
}

func TestVersionNext_InterruptedExitsWith128PlusTheSignal(t *testing.T) {
	tests := []struct {
		sig  os.Signal
		code int
	}{
		{syscall.SIGINT, 130},
		{syscall.SIGTERM, 143},
		{syscall.Signal(1), 129}, // SIGHUP; syscall.SIGHUP does not exist on Windows
		{nil, 2},                 // cancelled without a signal
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.sig), func(t *testing.T) {
			f := newNextFixture(t, "")
			cancelled, cancel := context.WithCancel(context.Background())
			cancel()
			f.signalCtx, f.signal = cancelled, tt.sig

			code, stdout, stderr := f.run(t)

			if code != tt.code || stdout != "" || !strings.Contains(stderr, "interrupted") {
				t.Errorf("code=%d stdout=%q stderr=%q, want %d", code, stdout, stderr, tt.code)
			}
		})
	}
}

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

// flushFailingWriter accepts writes and fails its Flush.
type flushFailingWriter struct{ bytes.Buffer }

func (*flushFailingWriter) Flush() error { return errors.New("disk full") }

func TestVersionNext_WriteErrorsAreReported(t *testing.T) {
	for _, format := range versionFormats {
		for _, w := range []struct {
			name   string
			writer io.Writer
			want   string
		}{{"write", failingWriter{}, "broken pipe"}, {"flush", &flushFailingWriter{}, "disk full"}} {
			t.Run(format+"/"+w.name, func(t *testing.T) {
				f := newNextFixture(t, "")
				f.results = []versioning.Result{released("repo", "1.2.0", "")}
				f.stdout = w.writer

				code, _, stderr := f.run(t, "--format", format)

				if code != 2 || !strings.Contains(stderr, w.want) {
					t.Errorf("code=%d stderr=%q, want 2 naming %q", code, stderr, w.want)
				}
			})
		}
	}
}

func TestVersionNext_UsageShowsTheDefaultFetchTimeout(t *testing.T) {
	want := fmt.Sprintf("default %ds", int(releaseclone.DefaultFetchTimeout.Seconds()))
	if !strings.Contains(versionNextUsage, want) {
		t.Errorf("usage lacks %q", want)
	}
}

// ---------------------------------------------------------------------------
// The output shape follows the request, not the number of results.
// ---------------------------------------------------------------------------

const threeProjectsYAML = `release:
  projects:
    - name: core
      path: core
    - name: api
      path: api
      depends_on: [{ project: core }]
    - name: web
      path: web
`

const oneProjectYAML = `release:
  projects:
    - name: api
      path: .
`

func TestVersionNext_ProjectFlagPrintsOnlyThatProject(t *testing.T) {
	f := newNextFixture(t, threeProjectsYAML)
	// The engine also returns the dependency it computed for depends_on.
	f.results = []versioning.Result{released("core", "1.0.1", ""), released("api", "1.0.1", "")}

	_, stdout, _ := f.run(t, "--project", "api", "--json")
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got["project"] != "api" {
		t.Fatalf("stdout = %q (%v), want one object for api", stdout, err)
	}

	_, stdout, _ = f.run(t, "--project", "api", "--format", "env")
	values, _ := parseEnv(t, stdout)
	if len(values) != 12 || values["new_release_version"] != "1.0.1" {
		t.Errorf("env = %v, want the 12 unprefixed outputs of api", values)
	}

	_, stdout, _ = f.run(t, "--project", "api")
	if strings.Contains(stdout, "core") || !strings.Contains(stdout, "api (develop)") {
		t.Errorf("text = %q, want api only", stdout)
	}
}

func TestVersionNext_MonorepoIsAnArrayEvenWithOneResult(t *testing.T) {
	f := newNextFixture(t, twoProjectsYAML)
	f.results = []versioning.Result{released("api", "1.2.0", "")}

	_, stdout, _ := f.run(t, "--json")
	var got []versioning.Result
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || len(got) != 1 {
		t.Fatalf("stdout = %q (%v), want an array", stdout, err)
	}

	_, stdout, _ = f.run(t, "--format", "env")
	values, _ := parseEnv(t, stdout)
	if values["api_new_release_version"] != "1.2.0" {
		t.Errorf("env = %v, want prefixed keys", values)
	}
}

func TestVersionNext_SingleProjectSectionIsAnObject(t *testing.T) {
	f := newNextFixture(t, oneProjectYAML)
	f.results = []versioning.Result{released("api", "1.2.0", "")}

	_, stdout, _ := f.run(t, "--json")
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got["project"] != "api" {
		t.Fatalf("stdout = %q (%v), want one object", stdout, err)
	}
}

// Without a release: section the engine computes the whole repository as one
// project and names it itself; the CLI prints that single result whatever
// its name, so a path that resolves differently (symlinks, Windows case)
// cannot turn a computed version into "no result".
func TestVersionNext_NoSectionPrintsTheSingleResultWhateverItsName(t *testing.T) {
	f := newNextFixture(t, "")
	f.results = []versioning.Result{released("Other-Name", "1.0.0", "")}

	code, stdout, stderr := f.run(t, "--json")
	var got map[string]any
	if code != 0 || json.Unmarshal([]byte(stdout), &got) != nil || got["project"] != "Other-Name" {
		t.Fatalf("code=%d stdout=%q stderr=%q, want exit 0 and the single result", code, stdout, stderr)
	}
}

func TestVersionNext_MissingRequestedResultIsAnError(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		args    []string
		results []versioning.Result
	}{
		{"no section, no result", "", nil, nil},
		{"project flag, only its dependency", threeProjectsYAML, []string{"--project", "api"}, []versioning.Result{released("core", "1.0.1", "")}},
		{"single project, no result", oneProjectYAML, nil, nil},
		{"monorepo, no result", twoProjectsYAML, nil, nil},
	}
	for _, tt := range tests {
		for _, format := range versionFormats {
			t.Run(tt.name+"/"+format, func(t *testing.T) {
				f := newNextFixture(t, tt.yaml)
				f.results = tt.results

				code, stdout, stderr := f.run(t, append(tt.args, "--format", format)...)

				if code != 2 || stdout != "" || !strings.Contains(stderr, "no result") {
					t.Errorf("code=%d stdout=%q stderr=%q, want exit 2, nothing on stdout", code, stdout, stderr)
				}
			})
		}
	}
}

func TestVersionNext_SaysWhereTheVersionConfigCameFrom(t *testing.T) {
	tests := []struct {
		name, yaml, project string
		source, file        string
		want                string
	}{
		{"release section", oneProjectYAML, "api", "repository", ".ai-workflow/config.yaml", "version config: release: section (.ai-workflow/config.yaml)"},
		{"built-in", "", "repo", "default", "", "version config: built-in default (dim)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newNextFixture(t, tt.yaml)
			f.results = []versioning.Result{released(tt.project, "1.1.0", "")}
			code, stdout, _ := f.run(t)
			if code != 0 || !strings.Contains(stdout, tt.want) {
				t.Errorf("exit %d, stdout:\n%s", code, stdout)
			}
			code, stdout, _ = f.run(t, "--json")
			var got map[string]any
			if err := json.Unmarshal([]byte(stdout), &got); err != nil || code != 0 || got["semantic_release_config"] != tt.source {
				t.Errorf("json %v %v: %s", code, err, stdout)
			}
			if file, _ := got["semantic_release_config_file"].(string); file != tt.file {
				t.Errorf("semantic_release_config_file = %q, want %q", file, tt.file)
			}
		})
	}
}

// S9, S1: the 4.2.0 release: and pipeline: sections are refused before the
// engine runs, saying what replaced them.
func TestVersionNext_RefusesTheOldSections(t *testing.T) {
	for _, tt := range []struct{ yaml, want string }{
		{"release:\n  semantic_release: { version: 24.2.3 }\n  projects:\n    - name: api\n      path: .\n", "run nerv migrate"},
		{"pipeline:\n  test: { run: make test }\n", "validate and test: nerv release"},
	} {
		f := newNextFixture(t, tt.yaml)
		code, stdout, stderr := f.run(t)
		if code != 1 || stdout != "" || !strings.Contains(stderr, tt.want) || len(f.calls) != 0 {
			t.Errorf("%q: code=%d stdout=%q stderr=%q calls=%d", tt.yaml, code, stdout, stderr, len(f.calls))
		}
	}
}

// S3: without a release: section, a semantic-release configuration the engine
// does not read is pointed out (the defaults are used).
func TestVersionNext_WarnsAboutAnUnimportedSemanticReleaseConfig(t *testing.T) {
	f := newNextFixture(t, "")
	if err := os.WriteFile(filepath.Join(f.top, ".releaserc.json"), []byte(`{"branches":["main"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f.results = []versioning.Result{notReleased("repo")}
	code, stdout, stderr := f.run(t)
	if code != 0 || !strings.Contains(stderr, "nerv release setup --import") || strings.Contains(stdout, "setup") {
		t.Errorf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// With a release: section there is nothing to warn about.
	g := newNextFixture(t, oneProjectYAML)
	if err := os.WriteFile(filepath.Join(g.top, ".releaserc.json"), []byte(`{"branches":["main"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g.results = []versioning.Result{notReleased("api")}
	if _, _, stderr := g.run(t); strings.Contains(stderr, "setup --import") {
		t.Errorf("warned with a release: section: %q", stderr)
	}
}

// A release: section without projects configures the engine for the whole
// repository: one result, its settings reach the engine.
func TestVersionNext_ReleaseSectionWithoutProjects(t *testing.T) {
	f := newNextFixture(t, "release:\n  providers:\n    conventional-commits:\n      tag_format: release-${version}\n")
	f.results = []versioning.Result{released("repo", "1.2.0", "")}
	code, stdout, stderr := f.run(t, "--json")
	var got map[string]any
	if code != 0 || json.Unmarshal([]byte(stdout), &got) != nil || got["semantic_release_config"] != "repository" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if len(f.calls) != 1 || f.calls[0].Engine.Config()["tag_format"] != "release-${version}" {
		t.Errorf("engine config = %+v", f.calls)
	}
}

// S4, S7: release.version selects the version engine; an unknown one, a
// key the engine does not declare, a refusal of the engine's own validate
// and the old engine keys under release: are refused (exit 1) before the
// engine computes anything.
func TestVersionNext_RefusesAnInvalidVersionEngine(t *testing.T) {
	for _, tt := range []struct{ name, yaml, want string }{
		{"unknown engine", "release:\n  version: semver\n", `release.version: unknown version provider "semver" (want conventional-commits)`},
		{"unknown key", "release:\n  providers:\n    conventional-commits: { tagformat: v1 }\n", "release.providers.conventional-commits.tagformat: unknown key (want branches, release_rules, note_keywords, tag_format, repository_url, changelog)"},
		{"engine refusal", "release:\n  providers:\n    conventional-commits: { tag_format: v1 }\n", `release.providers.conventional-commits.tag_format: "v1" must contain ${version} exactly once`},
		{"old engine key", "release:\n  tag_format: v${version}\n", "release.tag_format: moved to release.providers.conventional-commits.tag_format"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newNextFixture(t, tt.yaml)
			code, stdout, stderr := f.run(t)
			if code != 1 || stdout != "" || !strings.Contains(stderr, tt.want) || len(f.calls) != 0 {
				t.Errorf("exit %d, calls %d, stdout %q\nstderr %s\nwant %q", code, len(f.calls), stdout, stderr, tt.want)
			}
		})
	}
}

// version next checks the build part of release: as it did before the
// build providers: an unknown build provider, a build provider's own
// validate refusing, and a key the provider does not declare are refused
// (exit 1) before the engine computes anything, so /release:setup's check
// with version next catches them.
func TestVersionNext_RefusesAnInvalidBuildProvider(t *testing.T) {
	for _, tt := range []struct{ name, yaml, want string }{
		{"unknown provider", "release:\n  build: maven\n  projects:\n    - { name: app, path: . }\n", `unknown build provider "maven"`},
		{"provider refusal", "release:\n  build: docker-compose\n  projects:\n    - { name: app, path: . }\n", "release.projects[0]"},
		{"unknown key", "release:\n  build: bash\n  providers:\n    bash: { bogus: x }\n  projects:\n    - { name: app, path: . }\n", "release.providers.bash.bogus: unknown key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newNextFixture(t, tt.yaml)
			code, stdout, stderr := f.run(t)
			if code != 1 || stdout != "" || !strings.Contains(stderr, tt.want) || len(f.calls) != 0 {
				t.Errorf("exit %d, calls %d, stdout %q\nstderr %s\nwant %q", code, len(f.calls), stdout, stderr, tt.want)
			}
		})
	}
}
