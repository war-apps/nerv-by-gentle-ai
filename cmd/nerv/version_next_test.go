package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// shipconfigValidationError is a release configuration problem as the engine
// reports it.
var shipconfigValidationError = shipconfig.ValidationError{Problems: []string{"release.projects[0].path: required"}}

// nextFixture is a fake repository for `nerv version next`: git's top-level
// answer comes from a scripted runner, the release configuration (if any)
// from a real .nerv/nerv.yaml in a temp dir, and the engine is a recorded
// fake.
type nextFixture struct {
	top     string
	runner  *envtest.FakeRunner
	calls   []semrel.Options
	ctxErrs []error
	results []semrel.ProjectResult
	err     error
	// stopped counts the cancel functions the signal seam handed out that
	// were called.
	stopped int
	// signalCtx, when set, replaces the context the signal seam returns.
	signalCtx context.Context
}

func newNextFixture(t *testing.T, nervYAML string) *nextFixture {
	t.Helper()
	top := t.TempDir()
	if nervYAML != "" {
		if err := os.MkdirAll(filepath.Join(top, ".nerv"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(top, ".nerv", "nerv.yaml"), []byte(nervYAML), 0o644); err != nil {
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
	opts.ComputeVersions = func(ctx context.Context, o semrel.Options) ([]semrel.ProjectResult, error) {
		f.calls = append(f.calls, o)
		f.ctxErrs = append(f.ctxErrs, ctx.Err())
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return f.results, f.err
	}
	opts.SignalContext = func(parent context.Context) (context.Context, context.CancelFunc) {
		ctx := parent
		if f.signalCtx != nil {
			ctx = f.signalCtx
		}
		ctx, cancel := context.WithCancel(ctx)
		return ctx, func() { f.stopped++; cancel() }
	}
	return opts
}

func (f *nextFixture) run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(append([]string{"version", "next"}, args...), &out, &errOut, f.options(t))
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

func released(project, version, channel string) semrel.ProjectResult {
	parts := strings.SplitN(strings.SplitN(version, "-", 2)[0], ".", 3)
	return semrel.ProjectResult{
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

func notReleased(project string) semrel.ProjectResult {
	return semrel.ProjectResult{
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
	f.results = []semrel.ProjectResult{released("api", "1.2.0", "")}

	code, _, stderr := f.run(t, "--cwd", "/work/repo/sub", "--branch", "develop", "--project", "api", "--offline", "--fetch-timeout", "15s")

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	want := semrel.Options{RepoRoot: f.top, Branch: "develop", Project: "api", Offline: true, FetchTimeout: 15 * time.Second}
	if len(f.calls) != 1 || !reflect.DeepEqual(f.calls[0], want) {
		t.Fatalf("engine options = %+v, want %+v", f.calls, want)
	}
}

func TestVersionNext_DefaultsReadOriginOnTheCurrentBranch(t *testing.T) {
	f := newNextFixture(t, "")
	f.results = []semrel.ProjectResult{notReleased("repo")}

	code, stdout, stderr := f.run(t)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	want := semrel.Options{RepoRoot: f.top}
	if len(f.calls) != 1 || !reflect.DeepEqual(f.calls[0], want) {
		t.Fatalf("engine options = %+v, want %+v (current branch, origin read, default timeout)", f.calls, want)
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
		f.results = []semrel.ProjectResult{released("repo", "1.2.0-alpha.2", "alpha")}

		code, stdout, stderr := f.run(t, args...)

		if code != 0 {
			t.Fatalf("%v: exit code = %d, stderr = %q", args, code, stderr)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("%v: stdout is not one JSON object: %v (%q)", args, err, stdout)
		}
		if len(got) != 14 {
			t.Errorf("%v: %d keys, want project, branch and the 12 outputs: %v", args, len(got), got)
		}
		if got["new_release_version"] != "1.2.0-alpha.2" || got["new_release_channel"] != "alpha" || got["new_release_published"] != true || got["project"] != "repo" || got["branch"] != "develop" {
			t.Errorf("%v: got %v", args, got)
		}
	}
}

func TestVersionNext_JSONSeveralProjectsIsAnArray(t *testing.T) {
	f := newNextFixture(t, twoProjectsYAML)
	f.results = []semrel.ProjectResult{released("api", "1.2.0", ""), notReleased("web-app")}

	code, stdout, _ := f.run(t, "--json")

	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	var got []semrel.ProjectResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not a JSON array: %v (%q)", err, stdout)
	}
	if len(got) != 2 || got[0] != f.results[0] || got[1] != f.results[1] {
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
	f.results = []semrel.ProjectResult{r}

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
	f.results = []semrel.ProjectResult{r}

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
	f.results = []semrel.ProjectResult{released("api", "1.2.0", ""), released("web-app", "0.3.1", "")}

	_, stdout, _ := f.run(t, "--format", "env")

	_, delims := parseEnv(t, stdout)
	if delims["api_new_release_notes"] == delims["web_app_new_release_notes"] {
		t.Errorf("both heredocs use delimiter %q", delims["api_new_release_notes"])
	}
}

func TestVersionNext_EnvSeveralProjectsArePrefixed(t *testing.T) {
	f := newNextFixture(t, twoProjectsYAML)
	f.results = []semrel.ProjectResult{released("api", "1.2.0", ""), notReleased("web-app")}

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
	f.results = []semrel.ProjectResult{released("api", "1.2.0-alpha.2", "alpha"), notReleased("web-app")}

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
	f.results = []semrel.ProjectResult{r}

	_, stdout, _ := f.run(t, "--format", "text")

	for _, want := range []string{"1.0.0", "default channel", "last release: none"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("text output lacks %q:\n%s", want, stdout)
		}
	}
}

func TestVersionNext_NoNewReleaseIsNotAnError(t *testing.T) {
	f := newNextFixture(t, "")
	f.results = []semrel.ProjectResult{notReleased("repo")}

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
			name: "node missing", code: 2,
			err:      errors.New("Node.js is required to compute versions with semantic-release (node was not found in PATH)"),
			contains: []string{"Node.js is required"},
			absent:   []string{"--offline"},
		},
		{
			name: "git too old", code: 2,
			err:      errors.New("git 2.30.0 is too old: selecting a project's commits by path needs git 2.31.0 or newer"),
			contains: []string{"git 2.30.0 is too old"},
		},
		{
			name: "origin unreachable", code: 2,
			err:      fmt.Errorf("reading release tags and notes from origin (https://***@github.com/x/y.git): %w: exit status 128", semrel.ErrOriginFetch),
			contains: []string{"origin", "--offline"},
		},
		{
			name: "validation error from the engine", code: 1,
			err:      fmt.Errorf("wrapped: %w", &shipconfigValidationError),
			contains: []string{"release.projects[0].path"},
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
	cancel() // as if SIGINT/SIGTERM/SIGHUP arrived
	f.signalCtx = cancelled

	code, stdout, stderr := f.run(t)

	if len(f.ctxErrs) != 1 || f.ctxErrs[0] == nil {
		t.Fatalf("the engine did not run under the signal context: %v", f.ctxErrs)
	}
	if code != 2 || stdout != "" || !strings.Contains(stderr, "interrupted") {
		t.Errorf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if f.stopped != 1 {
		t.Errorf("signal context released %d times, want 1", f.stopped)
	}
}

func TestVersionNext_ReleasesTheSignalContextOnSuccess(t *testing.T) {
	f := newNextFixture(t, "")
	f.results = []semrel.ProjectResult{notReleased("repo")}

	if code, _, _ := f.run(t); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if f.stopped != 1 {
		t.Errorf("signal context released %d times, want 1", f.stopped)
	}
}
