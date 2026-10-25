package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

func testOptions(home string) options {
	return options{
		PluginFS:   nerv.PluginFS(),
		Runner:     &envtest.FakeRunner{},
		Now:        func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) },
		LookPath:   func(string) (string, error) { return "", os.ErrNotExist },
		Stdin:      strings.NewReader(""),
		Home:       home,
		IsTerminal: func() bool { return false },
	}
}

// ---------------------------------------------------------------------------
// No mode flag, no --answers, stdin not a terminal: refusal notice, exit 1.
// ---------------------------------------------------------------------------

func TestRunConfigure_NoModeFlag_NotATerminal_PrintsNoticeAndExits1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure"}, &stdout, &stderr, testOptions(home))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "stdin is not a terminal") {
		t.Errorf("expected the not-a-terminal notice, got: %q", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// No mode flag, stdin IS a terminal: the wizard runs interactively.
// ---------------------------------------------------------------------------

func TestRunConfigure_NoModeFlag_Terminal_RunsWizard(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()
	opts := testOptions(home)
	opts.IsTerminal = func() bool { return true }
	// Real blank lines (not an exhausted reader — see wizard.ErrInputClosed)
	// for every user-config prompt, so every value keeps its current
	// default and the wizard completes instead of aborting.
	opts.Stdin = strings.NewReader(strings.Repeat("\n", 12))

	code := run([]string{
		"configure",
		"--skip-skills", "--skip-models", "--skip-repos", "--no-refresh",
	}, &stdout, &stderr, opts)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q, stdout=%q)", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "NERV Setup Wizard") {
		t.Errorf("expected the wizard banner, got: %q", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// --answers <file> drives the wizard end to end, regardless of terminal-ness.
// ---------------------------------------------------------------------------

func TestRunConfigure_Answers_DrivesWizard(t *testing.T) {
	home := t.TempDir()
	configPath := paths.Resolve(home).UserConfig
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("enabled: true\ngit:\n  base_branch: develop\n  worktree: ask\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	answersPath := filepath.Join(home, "answers.txt")
	answers := "release\n" + strings.Repeat("\n", 30)
	if err := os.WriteFile(answersPath, []byte(answers), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"configure", "--answers", answersPath,
		"--skip-skills", "--skip-models", "--skip-repos", "--no-refresh",
	}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q, stdout=%q)", code, stderr.String(), stdout.String())
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "base_branch: release") {
		t.Errorf("expected base_branch updated, got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// --answers pointing at a missing file behaves like an empty answers file:
// openAnswersFile still treats "file not found" as "no answers" rather
// than an error, but an empty answers file is now genuine EOF at the
// first prompt (P3.1.2), so the wizard aborts with exit 1 instead of
// silently keeping every default.
// ---------------------------------------------------------------------------

func TestRunConfigure_Answers_MissingFileActsEmpty(t *testing.T) {
	home := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"configure", "--answers", filepath.Join(home, "nonexistent-answers.txt"),
		"--skip-skills", "--skip-models", "--skip-repos", "--no-refresh",
	}, &stdout, &stderr, testOptions(home))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr=%q, stdout=%q)", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "input ended before the wizard finished; nothing was written") {
		t.Errorf("expected the ErrInputClosed notice, got: %q", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Mutually exclusive modes -> usage error, exit 2.
// ---------------------------------------------------------------------------

func TestRunConfigure_MultipleModes_UsageErrorExit2(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure", "--print", "--set", "git.worktree=ask"}, &stdout, &stderr, testOptions(home))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stderr.Len() == 0 {
		t.Error("expected usage text on stderr")
	}
}

// ---------------------------------------------------------------------------
// --print always emits JSON, regardless of --json.
// ---------------------------------------------------------------------------

func TestRunConfigure_Print_EmitsJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure", "--print"}, &stdout, &stderr, testOptions(home))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (stdout=%q)", err, stdout.String())
	}
	if _, ok := decoded["config_path"]; !ok {
		t.Error("missing config_path in JSON output")
	}
}

// ---------------------------------------------------------------------------
// --set happy path, then a repeated call with the same value is a no-op —
// changed true/backup set, then changed false/backup null.
// ---------------------------------------------------------------------------

func TestRunConfigure_Set_HappyPathThenNoop(t *testing.T) {
	home := t.TempDir()
	configPath := paths.Resolve(home).UserConfig
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("enabled: true\ngit:\n  worktree: ask\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout1, stderr1 bytes.Buffer
	code := run([]string{"configure", "--set", "git.worktree=always", "--json"}, &stdout1, &stderr1, testOptions(home))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr1.String())
	}
	var first struct {
		Changed bool    `json:"changed"`
		Backup  *string `json:"backup"`
	}
	if err := json.Unmarshal(stdout1.Bytes(), &first); err != nil {
		t.Fatalf("stdout1 not valid JSON: %v (%q)", err, stdout1.String())
	}
	if !first.Changed {
		t.Error("first call: changed = false, want true")
	}
	if first.Backup == nil {
		t.Error("first call: backup = nil, want a path")
	}

	var stdout2, stderr2 bytes.Buffer
	code = run([]string{"configure", "--set", "git.worktree=always", "--json"}, &stdout2, &stderr2, testOptions(home))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr2.String())
	}
	var second struct {
		Changed bool    `json:"changed"`
		Backup  *string `json:"backup"`
	}
	if err := json.Unmarshal(stdout2.Bytes(), &second); err != nil {
		t.Fatalf("stdout2 not valid JSON: %v (%q)", err, stdout2.String())
	}
	if second.Changed {
		t.Error("second call: changed = true, want false")
	}
	if second.Backup != nil {
		t.Errorf("second call: backup = %v, want nil", *second.Backup)
	}
}

func TestRunConfigure_Set_UnknownKeyExit1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure", "--set", "bogus.key=1"}, &stdout, &stderr, testOptions(home))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stdout=%q)", code, stdout.String())
	}
	if stdout.Len() == 0 {
		t.Error("expected a rejection message on stdout")
	}
}

// ---------------------------------------------------------------------------
// --home resolution failure (no override, HOME/USERPROFILE unset): exit 2.
// ---------------------------------------------------------------------------

func TestRunConfigure_HomeResolutionFailure_Exit2(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"configure", "--print"}, &stdout, &stderr, testOptions(""))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (stdout=%q)", code, stdout.String())
	}
}

// ---------------------------------------------------------------------------
// P3.1.3: "--home" is no longer a public flag on "nerv configure" — a fake
// home never sandboxes the global commands it can still reach (claude
// plugin, npx -g); the home directory comes only from options.Home
// (tests) or env.HomeDir() (production). Passing it is now an unknown
// flag, which fails the flag.ContinueOnError parse and exits 2.
// ---------------------------------------------------------------------------

func TestRunConfigure_HomeFlag_NoLongerAccepted(t *testing.T) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()

	code := run([]string{"configure", "--home", home, "--print"}, &stdout, &stderr, testOptions(home))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (a rejected unknown flag); stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

// tasksFixture is a user config.yml carrying a tasks: block written by an earlier
// release, with the Teamwork stages inline map and the settings of a provider
// that no longer exists. Core owns none of it and must keep every byte.
const tasksFixture = "enabled: true\n" +
	"git:\n" +
	"  base_branch: develop              # default base\n" +
	"  worktree: ask\n" +
	"tasks:\n" +
	"  provider: jira                    # teamwork | none\n" +
	"  rounding_minutes: 15\n" +
	"  providers:\n" +
	"    teamwork:\n" +
	"      assignee_id: 686035\n" +
	"      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }\n" +
	"    jira: { site: x }\n" +
	"artifacts:\n" +
	"  commit: at-close\n"

func tasksBlock(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, found := config.Parse(string(data)).Block("tasks")
	if !found {
		t.Fatalf("%s lost its tasks: block:\n%s", path, data)
	}
	return block
}

// S7: a full nerv configure round trip (--set, --set-model, the wizard and
// --init-repo over an existing project file) leaves a tasks: block's bytes
// exactly as they were.
func TestRunConfigure_RoundTrip_PreservesTasksBlock(t *testing.T) {
	home := t.TempDir()
	configPath := paths.Resolve(home).UserConfig
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(tasksFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	want, _ := config.Parse(tasksFixture).Block("tasks")

	answersPath := filepath.Join(home, "answers.txt")
	if err := os.WriteFile(answersPath, []byte("release\n"+strings.Repeat("\n", 30)), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"configure", "--set", "git.base_branch=main"},
		{"configure", "--set-model", "hyuga=opus/xhigh"},
		{"configure", "--answers", answersPath, "--skip-skills", "--skip-models", "--skip-repos", "--no-refresh"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr, testOptions(home)); code != 0 {
			t.Fatalf("%v: exit code = %d, want 0 (stdout=%q)", args, code, stdout.String())
		}
		if got := tasksBlock(t, configPath); got != want {
			t.Errorf("%v changed the tasks: block:\ngot:\n%s\nwant:\n%s", args, got, want)
		}
	}
	if data, _ := os.ReadFile(configPath); !strings.Contains(string(data), "base_branch: release") {
		t.Errorf("the round trip did not write its own change:\n%s", data)
	}

	repo := t.TempDir()
	projectPath := filepath.Join(repo, ".ai-workflow/config.yaml")
	if err := os.MkdirAll(filepath.Dir(projectPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte(tasksFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := testOptions(home)
	opts.Runner = &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"git -C " + repo + " rev-parse --show-toplevel": {Stdout: repo + "\n"},
	}}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"configure", "--init-repo", repo}, &stdout, &stderr, opts); code != 0 {
		t.Fatalf("--init-repo: exit code = %d, want 0 (stdout=%q)", code, stdout.String())
	}
	if data, _ := os.ReadFile(projectPath); string(data) != tasksFixture {
		t.Errorf("--init-repo rewrote an existing project file:\n%s", data)
	}
}

// S1: the task flags are gone; like any unknown flag they exit 2 with the
// usage text, and the usage no longer names them.
func TestRunConfigure_RemovedTaskFlags_Exit2(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	for _, args := range [][]string{
		{"--init-repo", repo, "--repo-provider", "teamwork"},
		{"--init-repo", repo, "--repo-project-id", "1"},
		{"--init-repo", repo, "--repo-tasklist-id", "1"},
		{"--install-commands"},
		{"--skip-commands", "--no-refresh"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(append([]string{"configure"}, args...), &stdout, &stderr, testOptions(home))
		if code != 2 {
			t.Errorf("%v: exit code = %d, want 2 (stdout=%q)", args, code, stdout.String())
		}
		if !strings.Contains(stderr.String(), "Usage: nerv configure") {
			t.Errorf("%v: expected the usage text on stderr, got %q", args, stderr.String())
		}
	}
	for _, removed := range []string{"--repo-provider ", "--repo-project-id", "--repo-tasklist-id", "--install-commands", "--skip-commands", "Teamwork"} {
		if strings.Contains(configureUsage, removed) {
			t.Errorf("configureUsage still mentions %q", removed)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".nerv")); err == nil {
		t.Error("a refused invocation wrote the repository config")
	}
}

func initRepoOptions(home, repo string) options {
	opts := testOptions(home)
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.Runner = &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"git -C " + repo + " rev-parse --show-toplevel": {Stdout: repo + "\n"},
	}}
	return opts
}

// S3: --repo-provider-config writes tasks.providers.<provider>.<key> into the
// new project file, keeping --repo-base and the rest of the file as before.
func TestRunConfigure_InitRepo_ProviderConfig_WritesProviderKeys(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run([]string{"configure", "--init-repo", repo, "--repo-base", "main",
		"--repo-provider-config", "teamwork.project_id=111",
		"--repo-provider-config", "teamwork.tasklist_id=222",
	}, &stdout, &stderr, initRepoOptions(home, repo))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(repo, ".ai-workflow/config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	doc := config.Parse(string(data))
	for key, want := range map[string]string{
		"enabled":                              "true",
		"git.base_branch":                      "main",
		"tasks.providers.teamwork.project_id":  "111",
		"tasks.providers.teamwork.tasklist_id": "222",
	} {
		if got, _ := doc.Scalar(key); got != want {
			t.Errorf("%s = %q, want %q:\n%s", key, got, want, data)
		}
	}
	if _, found := doc.Scalar("tasks.provider"); found {
		t.Errorf("tasks.provider written without being asked:\n%s", data)
	}
}

// S3: --repo-provider-config is refused (exit 1, naming the problem, writing
// nothing) for a provider with no embedded manifest, a key outside its
// config, or a value that is not valid for the key's type; used without
// --init-repo it is a usage error.
func TestRunConfigure_InitRepo_ProviderConfig_Refusals(t *testing.T) {
	cases := []struct {
		entry, want string
	}{
		{"jira.site=x", "Unknown task provider 'jira'"},
		{"teamwork.bogus=1", "Unknown key 'bogus' for task provider 'teamwork'"},
		{"teamwork.project_id=abc", "Invalid value 'abc' for tasks.providers.teamwork.project_id: expected int"},
		{"teamwork.billable=maybe", "Invalid value 'maybe' for tasks.providers.teamwork.billable: expected bool"},
		{"teamwork.stages=x", "Invalid value 'x' for tasks.providers.teamwork.stages: expected map"},
		{"teamwork=1", "Invalid --repo-provider-config entry 'teamwork=1'"},
		{"teamwork.project_id", "Invalid --repo-provider-config entry 'teamwork.project_id'"},
	}
	for _, tc := range cases {
		t.Run(tc.entry, func(t *testing.T) {
			home := t.TempDir()
			repo := t.TempDir()
			var stdout, stderr bytes.Buffer
			code := run([]string{"configure", "--init-repo", repo, "--repo-provider-config", tc.entry}, &stdout, &stderr, initRepoOptions(home, repo))
			if code != 1 {
				t.Fatalf("exit code = %d, want 1 (stdout=%q)", code, stdout.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.want)
			}
			if _, err := os.Stat(filepath.Join(repo, ".nerv")); err == nil {
				t.Error("a refused entry wrote the repository config")
			}
		})
	}

	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{"configure", "--set", "git.worktree=ask", "--repo-provider-config", "teamwork.project_id=1"}, &stdout, &stderr, initRepoOptions(home, home))
	if code != 2 {
		t.Errorf("without --init-repo: exit code = %d, want 2 (stdout=%q)", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "--repo-provider-config requires --init-repo") {
		t.Errorf("stderr = %q, want the --init-repo requirement", stderr.String())
	}
}
