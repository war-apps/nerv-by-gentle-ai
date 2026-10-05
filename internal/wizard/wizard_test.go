package wizard_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/wizard"
)

// fixtureLF mirrors the real nerv.yaml shape (every managed key at its
// built-in default), LF line endings — port of tests/configure.test.ps1's
// $fixtureLf, trimmed to the fields the Go wizard's User-config section
// actually asks about (the exotic known_projects:/sources:/sources_howto:
// preservation is already covered by internal/config's own P1a suite; this
// file exercises the wizard's own prompt flow and section wiring).
const fixtureLF = "" +
	"enabled: true\n" +
	"skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md\n" +
	"  testing: [tdd, playwright-best-practices]                        # ritsuko, kaworu, maya\n" +
	"  code: [dotnet-best-practices, typescript-best-practices]         # pilots\n" +
	"  best-practices: [best-practices, solid-principles, clean-code-guard]  # balthasar\n" +
	"  architecture: [hexagonal-architecture, c4-architecture]          # melchor\n" +
	"  audit: [security-review, clean-code-guard]                       # kaji passes\n" +
	"critical_paths: [auth/, payments/, migrations/, infra/]            # Hyuga auto-critical\n" +
	"git:\n" +
	"  base_branch: develop              # default base for the worktree offer\n" +
	"  worktree: ask                     # ask | always | never\n" +
	`  worktree_pattern: ".claude/worktrees/{slug}"   # where task worktrees are created; {slug} {branch} {prefix} {id}` + "\n" +
	`  branch_pattern: "feature/{prefix}-{id}-{slug}"   # prefix comes from the provider (tw)` + "\n" +
	`  commit_ref_pattern: "({PREFIX}-{id})"` + "\n" +
	"tasks:\n" +
	`  provider: teamwork                # teamwork | none ; "ask" when absent` + "\n" +
	"  ask_when_missing: true            # preflight asks task + worktree + branch if no active task\n" +
	"  subtasks_per_wave: false\n" +
	"  timer_store: ~/.claude/work/timers.json\n" +
	"  rounding_minutes: 15\n" +
	"  providers:                        # one block per provider, only the enabled one is required\n" +
	"    teamwork:\n" +
	"      task_ref_prefix: tw           # {prefix} in branch_pattern / commit_ref_pattern\n" +
	"      assignee_id: 686035           # user scope\n" +
	"      default_project_id: 1271726\n" +
	"      default_tasklist_id: 3951970\n" +
	"      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }\n" +
	"artifacts:\n" +
	"  commit: at-close                  # with-change | at-close | never (default: at-close)\n"

func fixedNow() time.Time { return time.Date(2026, 9, 29, 15, 4, 5, 0, time.UTC) }

func lookPathNone(string) (string, error) { return "", os.ErrNotExist }

// lookPathHerdr fakes herdr present on PATH, everything else absent.
func lookPathHerdr(name string) (string, error) {
	if name == "herdr" {
		return "/usr/local/bin/herdr", nil
	}
	return "", os.ErrNotExist
}

// herdrGetenv fakes the environment lookup the git.worktree_pattern menu's
// herdr detection uses: only APPDATA resolves (to appData), matching the
// Windows config-path branch; every other name is unset.
func herdrGetenv(appData string) func(string) string {
	return func(name string) string {
		if name == "APPDATA" {
			return appData
		}
		return ""
	}
}

// writeHerdrConfig writes a herdr config.toml under whichever location
// internal/herdr.Detect resolves for runtime.GOOS on this test host,
// mirroring its own config-path rule instead of assuming one OS.
func writeHerdrConfig(t *testing.T, home, appData, content string) {
	t.Helper()
	var path string
	if runtime.GOOS == "windows" {
		path = filepath.Join(appData, "herdr", "config.toml")
	} else {
		path = filepath.Join(home, ".config", "herdr", "config.toml")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func noRunner() *envtest.FakeRunner { return &envtest.FakeRunner{} }

func testPaths(root, home string) configure.Paths {
	return configure.Paths{
		Config:      filepath.Join(root, "nerv.yaml"),
		State:       filepath.Join(home, ".gentle-ai", "state.json"),
		SkillsDir:   filepath.Join(home, ".claude", "skills"),
		CommandsDir: filepath.Join(home, ".claude", "commands", "task"),
	}
}

func writeFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(fixtureLF), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skipAll() wizard.Options {
	return wizard.Options{SkipSkills: true, SkipModels: true, SkipRepos: true, SkipCommands: true, NoRefresh: true}
}

func countBackups(t *testing.T, root string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "nerv.yaml.bak-configure-*"))
	if err != nil {
		t.Fatal(err)
	}
	return len(matches)
}

// ---------------------------------------------------------------------------
// Case group F (tests/configure.test.ps1's ~403-424): every prompt
// answered with an actual blank line -> every prompt keeps its current
// value, the config is left byte-identical, and no backup is written.
//
// A genuinely empty reader is a different case now (P3.1.2's EOF-aborts
// fix): see TestRun_ReaderEndsAtFirstPrompt_AbortsWithoutWriteOrRunnerCall
// below. Blank line and EOF used to be indistinguishable (both fell
// through readLineOr's own default); this test used to exercise that by
// passing a wholly empty reader, which is exactly the incident the fix
// addresses — it now needs real blank lines instead.
// ---------------------------------------------------------------------------

func TestRun_AllBlankAnswers_ByteIdenticalNoBackup(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}

	opts := skipAll()
	opts.Paths = paths

	// git(5) + tasks(5) + tasks.providers.teamwork(11, since the fixture's
	// provider stays "teamwork" on a blank answer) + skills(5) +
	// critical_paths(1) + artifacts.commit(1) = 28 prompts, every one a
	// real blank line (not exhaustion).
	in := strings.NewReader(strings.Repeat("\n", 28))

	var out bytes.Buffer
	summary, err := wizard.Run(deps, in, &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if summary.Changed {
		t.Errorf("Changed = true, want false; output:\n%s", out.String())
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fixtureLF {
		t.Errorf("config mutated on all-blank answers:\ngot:\n%s\nwant:\n%s", got, fixtureLF)
	}
	if n := countBackups(t, root); n != 0 {
		t.Errorf("backups = %d, want 0", n)
	}
	if strings.Contains(out.String(), "Restart Claude Code") {
		t.Error("expected no restart reminder when nothing changed")
	}
}

// ---------------------------------------------------------------------------
// P3.1.2 (EOF aborts): a reader that ends at the very first prompt (no
// lines at all, i.e. immediate EOF — the same shape a redirected
// /dev/null gives the wizard) must abort with ErrInputClosed, write
// nothing, write no backup, and never call the process runner. This is
// the exact incident case: `nerv configure --home <tmp> < /dev/null`.
// ---------------------------------------------------------------------------

func TestRun_ReaderEndsAtFirstPrompt_AbortsWithoutWriteOrRunnerCall(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	runner := &envtest.FakeRunner{}
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathNone}

	opts := skipAll()
	opts.Paths = paths

	var out bytes.Buffer
	_, err := wizard.Run(deps, strings.NewReader(""), &out, opts)
	if !errors.Is(err, wizard.ErrInputClosed) {
		t.Fatalf("Run() error = %v, want ErrInputClosed", err)
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fixtureLF {
		t.Errorf("config mutated despite EOF abort:\ngot:\n%s\nwant:\n%s", got, fixtureLF)
	}
	if n := countBackups(t, root); n != 0 {
		t.Errorf("backups = %d, want 0", n)
	}
	if calls := mutatingCalls(runner.Calls); len(calls) != 0 {
		t.Errorf("mutating calls = %+v, want none: an aborted wizard must never touch the process runner", calls)
	}
}

// ---------------------------------------------------------------------------
// P3.1.2 (EOF aborts): a reader that answers every user-config prompt
// (with real blank lines, keeping every value unchanged, so section 1
// never even reaches its own write-confirmation) but ends exactly at the
// wizard's final "apply and refresh" gate must abort there too: nothing
// gets applied to the plugin cache and the process runner is never
// called, even though many earlier prompts were already answered —
// the same all-or-nothing guarantee, exercised at the very end of the
// wizard instead of the very start.
// ---------------------------------------------------------------------------

func TestRun_ReaderEndsAtApplyAndRefreshPrompt_AbortsWithoutRunnerCall(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	runner := &envtest.FakeRunner{}
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathNone}

	opts := wizard.Options{Paths: paths, SkipSkills: true, SkipModels: true, SkipRepos: true, SkipCommands: true}

	// The 28 user-config prompts, every one a real blank line (no
	// change, no write-confirmation asked) — then nothing left for the
	// "Apply models to the plugin cache and refresh it now?" gate.
	in := strings.NewReader(strings.Repeat("\n", 28))

	var out bytes.Buffer
	_, err := wizard.Run(deps, in, &out, opts)
	if !errors.Is(err, wizard.ErrInputClosed) {
		t.Fatalf("Run() error = %v, want ErrInputClosed; output:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Apply models to the plugin cache and refresh it now?") {
		t.Fatalf("expected the reader to have reached the apply-and-refresh prompt; output:\n%s", out.String())
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fixtureLF {
		t.Errorf("config mutated despite EOF abort:\ngot:\n%s\nwant:\n%s", got, fixtureLF)
	}
	if calls := mutatingCalls(runner.Calls); len(calls) != 0 {
		t.Errorf("mutating calls = %+v, want none: install.ApplyModels/RefreshCache must never run", calls)
	}
}

// ---------------------------------------------------------------------------
// P3.1.3 (models section custom-id EOF): the models section's "6 custom
// model id" sub-loop used to read with s.prompt, which silently yields ""
// on a genuinely exhausted reader instead of signalling EOF — and "" never
// matches customModelIDRe, so the loop spun forever instead of aborting
// like every other prompt in the wizard. Guarded with a timeout so a
// regression fails this test instead of hanging the whole suite.
// ---------------------------------------------------------------------------

func TestRun_ModelsSection_CustomModelID_EOFAfterPrompt_AbortsWithoutWriteOrRunnerCall(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	runner := &envtest.FakeRunner{}
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathNone}

	opts := wizard.Options{Paths: paths, SkipSkills: true, SkipRepos: true, SkipCommands: true, NoRefresh: true}

	var out bytes.Buffer
	in := modelsInput(&out,
		"balthasar", // single role
		"6",         // model choice: custom id
		// reader ends here: the custom-id sub-prompt gets genuine EOF
	)

	done := make(chan error, 1)
	go func() {
		_, err := wizard.Run(deps, in, &out, opts)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, wizard.ErrInputClosed) {
			t.Fatalf("Run() error = %v, want ErrInputClosed; output:\n%s", err, out.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() hung on the custom model id prompt after EOF; want ErrInputClosed")
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fixtureLF {
		t.Errorf("config mutated despite EOF abort:\ngot:\n%s\nwant:\n%s", got, fixtureLF)
	}
	if n := countBackups(t, root); n != 0 {
		t.Errorf("backups = %d, want 0", n)
	}
	if calls := mutatingCalls(runner.Calls); len(calls) != 0 {
		t.Errorf("mutating calls = %+v, want none: an aborted wizard must never touch the process runner", calls)
	}
}

// P3.1.3 (models section custom-id re-prompt): an invalid custom model id
// (not matching ^claude-.+$) must still re-prompt instead of aborting, and
// a later valid id must be applied as the override — proving the EOF fix
// above didn't break the existing re-prompt behavior.
func TestRun_ModelsSection_CustomModelID_InvalidThenValid_AppliesOverride(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}

	opts := wizard.Options{Paths: paths, SkipSkills: true, SkipRepos: true, SkipCommands: true, NoRefresh: true}

	var out bytes.Buffer
	in := modelsInput(&out,
		"balthasar",   // single role
		"6",           // model choice: custom id
		"not-a-model", // invalid: doesn't match ^claude-.+$, re-prompts
		"claude-x",    // valid custom id
		"",            // effort: blank keeps current
		"done",        // finish the role loop
		"y",           // write confirm
	)
	summary, err := wizard.Run(deps, in, &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if !summary.Changed {
		t.Fatalf("Changed = false, want true; output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Invalid model id") {
		t.Errorf("expected the invalid id to be rejected and re-prompted; output:\n%s", out.String())
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	want := "balthasar: { model: claude-x }"
	if !strings.Contains(string(got), want) {
		t.Errorf("expected %q in models: block:\n%s", want, got)
	}
}

// ---------------------------------------------------------------------------
// Case group G (tests/configure.test.ps1's ~426-548): only git.base_branch
// answered -> exactly one line differs, plus a repo init and a slash
// commands install driven by the same answers file.
// ---------------------------------------------------------------------------

func TestRun_BaseBranchChange_RepoInitAndCommandsInstall(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)

	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &envtest.FakeRunner{
		Responses: map[string]envtest.Response{
			"git -C " + repoDir + " rev-parse --show-toplevel": {Stdout: repoDir + "\n", ExitCode: 0},
		},
	}
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathNone}

	opts := wizard.Options{Paths: paths, SkipSkills: true, SkipModels: true, NoRefresh: true}

	lines := []string{"develop2"}
	for i := 0; i < 27; i++ { // worktree..artifacts.commit: 27 more blanks
		lines = append(lines, "")
	}
	lines = append(lines,
		"",      // write confirm [Y/n] -> blank means yes
		repoDir, // repo path
		"",      // repo base branch (keep)
		"",      // repo task provider (keep teamwork)
		"111",   // repo project id
		"222",   // repo tasklist id
		"",      // repo path again -> skip
		"y",     // install slash commands
	)
	in := strings.NewReader(strings.Join(lines, "\n"))

	var out bytes.Buffer
	summary, err := wizard.Run(deps, in, &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if !summary.Changed {
		t.Errorf("Changed = false, want true; output:\n%s", out.String())
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "base_branch: develop2") {
		t.Errorf("base_branch not updated:\n%s", got)
	}
	if diff := diffLineCount(fixtureLF, string(got)); diff != 1 {
		t.Errorf("diff line count = %d, want 1; got:\n%s", diff, got)
	}
	if n := countBackups(t, root); n < 1 {
		t.Error("expected a backup to be written")
	}

	repoConfigPath := filepath.Join(repoDir, ".nerv", "nerv.yaml")
	repoConfig, err := os.ReadFile(repoConfigPath)
	if err != nil {
		t.Fatalf("repo config not written: %v", err)
	}
	for _, want := range []string{"enabled: true", "project_id: 111", "tasklist_id: 222"} {
		if !strings.Contains(string(repoConfig), want) {
			t.Errorf("repo config missing %q:\n%s", want, repoConfig)
		}
	}

	copied, err := filepath.Glob(filepath.Join(paths.CommandsDir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(copied) == 0 {
		t.Error("expected the Teamwork /task:* commands to be copied")
	}
}

// ---------------------------------------------------------------------------
// A declined write confirmation leaves the file untouched even when an
// answer actually changed a value.
// ---------------------------------------------------------------------------

func TestRun_DeclineWrite_LeavesFileUntouched(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}

	opts := skipAll()
	opts.Paths = paths

	lines := []string{"develop2"}
	for i := 0; i < 27; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, "n") // decline the write

	var out bytes.Buffer
	summary, err := wizard.Run(deps, strings.NewReader(strings.Join(lines, "\n")), &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if summary.Changed {
		t.Error("Changed = true, want false (write declined)")
	}
	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fixtureLF {
		t.Errorf("config mutated despite declined write:\n%s", got)
	}
	if !strings.Contains(out.String(), "Aborted; no changes written to user config.") {
		t.Errorf("expected the abort message; output:\n%s", out.String())
	}
}

// ---------------------------------------------------------------------------
// Changing tasks.provider away from "teamwork" must skip every
// tasks.providers.teamwork.* prompt (11 fewer answer lines needed).
// ---------------------------------------------------------------------------

func TestRun_NonTeamworkProvider_SkipsTeamworkPrompts(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}

	opts := skipAll()
	opts.Paths = paths

	lines := []string{
		"",                           // git.base_branch
		"",                           // git.worktree
		"2",                          // git.worktree_pattern menu: custom (no herdr row: lookPathNone)
		".claude/worktrees/{branch}", // git.worktree_pattern custom value (CHANGED)
		"",                           // git.branch_pattern
		"",                           // git.commit_ref_pattern
		"none",                       // tasks.provider (CHANGED away from teamwork)
		"",                           // tasks.ask_when_missing
		"",                           // tasks.subtasks_per_wave
		"",                           // tasks.timer_store
		"",                           // tasks.rounding_minutes
		// no teamwork.* lines here
		"", "", "", "", "", // skills x5
		"",  // critical_paths
		"",  // artifacts.commit
		"y", // write confirm
	}

	var out bytes.Buffer
	summary, err := wizard.Run(deps, strings.NewReader(strings.Join(lines, "\n")), &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if !summary.Changed {
		t.Fatalf("Changed = false, want true; output:\n%s", out.String())
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "provider: none") {
		t.Errorf("tasks.provider not updated:\n%s", got)
	}
	// Every skills category and critical_paths/artifacts must be unchanged
	// (their blank answers were correctly consumed, not shifted by 11
	// unexpectedly-asked teamwork lines).
	for _, want := range []string{
		"testing: [tdd, playwright-best-practices]",
		"audit: [security-review, clean-code-guard]",
		"critical_paths: [auth/, payments/, migrations/, infra/]",
		"commit: at-close",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q preserved:\n%s", want, got)
		}
	}
	// the new git.worktree_pattern key must land in the file when answered.
	if !strings.Contains(string(got), `worktree_pattern: ".claude/worktrees/{branch}"`) {
		t.Errorf("expected worktree_pattern to be updated:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// git.worktree_pattern menu: herdr detected renders default/herdr/custom
// rows in that order, the resolved rows each with an "e.g." example, and
// choosing herdr (row 2) writes its resolved pattern.
// ---------------------------------------------------------------------------

func TestRun_WorktreePatternMenu_HerdrDetected_ChoosesHerdr(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	appData := filepath.Join(home, "appdata")
	writeHerdrConfig(t, home, appData, "[worktrees]\ndirectory = 'D:\\.worktrees'\n")
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathHerdr, Getenv: herdrGetenv(appData)}

	opts := skipAll()
	opts.Paths = paths

	lines := make([]string, 28)
	lines[2] = "2" // git.worktree_pattern menu: herdr row (default=1, herdr=2, custom=3)
	lines = append(lines, "y")

	var out bytes.Buffer
	summary, err := wizard.Run(deps, strings.NewReader(strings.Join(lines, "\n")), &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if !summary.Changed {
		t.Fatalf("Changed = false, want true; output:\n%s", out.String())
	}

	output := out.String()
	wantOrder := []string{"1) default", "2) herdr", "3) custom"}
	lastIdx := -1
	for _, want := range wantOrder {
		idx := strings.Index(output, want)
		if idx < 0 {
			t.Fatalf("expected %q in menu output:\n%s", want, output)
		}
		if idx < lastIdx {
			t.Fatalf("expected %q to appear after the previous row:\n%s", want, output)
		}
		lastIdx = idx
	}
	for _, want := range []string{
		"e.g. <repo-root>/.claude/worktrees/feature-tw-123-add-button",
		`e.g. D:\.worktrees\my-repo\feature-tw-123-add-button`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in menu output:\n%s", want, output)
		}
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `worktree_pattern: "D:\.worktrees\{repo}\{slug}"`) {
		t.Errorf("expected herdr pattern written:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// git.worktree_pattern menu: herdr absent shows only default/custom (no
// herdr row, custom shifts to "2"); choosing custom falls through to the
// free-text prompt.
// ---------------------------------------------------------------------------

func TestRun_WorktreePatternMenu_HerdrAbsent_ChoosesCustom(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}

	opts := skipAll()
	opts.Paths = paths

	lines := []string{
		"",                   // git.base_branch
		"",                   // git.worktree
		"2",                  // git.worktree_pattern menu: custom (no herdr row)
		"~/wt/{repo}/{slug}", // git.worktree_pattern custom value (CHANGED)
	}
	for i := 0; i < 25; i++ { // branch_pattern..artifacts.commit
		lines = append(lines, "")
	}
	lines = append(lines, "y")

	var out bytes.Buffer
	summary, err := wizard.Run(deps, strings.NewReader(strings.Join(lines, "\n")), &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if !summary.Changed {
		t.Fatalf("Changed = false, want true; output:\n%s", out.String())
	}

	output := out.String()
	if strings.Contains(output, "herdr") {
		t.Errorf("expected no herdr row when herdr is absent; output:\n%s", output)
	}
	if !strings.Contains(output, "2) custom") {
		t.Errorf("expected custom to be row 2 when herdr is absent; output:\n%s", output)
	}
	if !strings.Contains(output, "Choice [1-2, blank keeps current]:") {
		t.Errorf("expected a 2-row choice prompt; output:\n%s", output)
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `worktree_pattern: "~/wt/{repo}/{slug}"`) {
		t.Errorf("expected custom pattern written:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// git.worktree_pattern menu: a blank answer keeps the current value,
// exactly like every other user-config field.
// ---------------------------------------------------------------------------

func TestRun_WorktreePatternMenu_Blank_KeepsCurrent(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}

	opts := skipAll()
	opts.Paths = paths

	lines := []string{"develop2"} // git.base_branch (CHANGED, so the run still writes)
	for i := 0; i < 27; i++ {     // git.worktree..artifacts.commit, all blank
		lines = append(lines, "")
	}
	lines = append(lines, "y")

	var out bytes.Buffer
	summary, err := wizard.Run(deps, strings.NewReader(strings.Join(lines, "\n")), &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if !summary.Changed {
		t.Fatalf("Changed = false, want true; output:\n%s", out.String())
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `worktree_pattern: ".claude/worktrees/{slug}"`) {
		t.Errorf("expected worktree_pattern unchanged on blank:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// git.worktree_pattern menu: choosing "1" (default) writes the catalogue
// default, overwriting whatever pattern was previously configured.
// ---------------------------------------------------------------------------

func TestRun_WorktreePatternMenu_ChoosesDefault(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	customized := strings.Replace(fixtureLF,
		`worktree_pattern: ".claude/worktrees/{slug}"`,
		`worktree_pattern: ".claude/worktrees/{branch}"`, 1)
	if customized == fixtureLF {
		t.Fatal("fixture replacement did not match; fixtureLF's worktree_pattern line changed shape")
	}
	if err := os.WriteFile(filepath.Join(root, "nerv.yaml"), []byte(customized), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := testPaths(root, home)
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}

	opts := skipAll()
	opts.Paths = paths

	lines := make([]string, 28)
	lines[2] = "1" // git.worktree_pattern menu: default row
	lines = append(lines, "y")

	var out bytes.Buffer
	summary, err := wizard.Run(deps, strings.NewReader(strings.Join(lines, "\n")), &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if !summary.Changed {
		t.Fatalf("Changed = false, want true; output:\n%s", out.String())
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `worktree_pattern: ".claude/worktrees/{slug}"`) {
		t.Errorf("expected the catalogue default to be written:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Models section: the "magi" group keyword assigns model+effort to all
// three MAGI roles (balthasar, melchor, casper) in one answer.
// ---------------------------------------------------------------------------

func TestRun_ModelsSection_MagiGroup(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}

	opts := wizard.Options{Paths: paths, SkipSkills: true, SkipRepos: true, SkipCommands: true, NoRefresh: true}

	var out bytes.Buffer
	in := modelsInput(&out,
		"magi", // role group
		"2",    // model choice: opus
		"3",    // effort choice: high
		"done", // finish the role loop
		"y",    // write confirm
	)
	summary, err := wizard.Run(deps, in, &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if !summary.Changed {
		t.Fatalf("Changed = false, want true; output:\n%s", out.String())
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range config.Roles().Groups["magi"] {
		want := role + ": { model: opus, effort: high }"
		if !strings.Contains(string(got), want) {
			t.Errorf("expected %q in models: block:\n%s", want, got)
		}
	}
}

// modelsOffer is the prompt that opens the models section.
const modelsOffer = "Configure per-role model and effort now?"

// maxBlankAnswers bounds how many blank answers modelsInput gives before the
// models offer appears, so a missing or renamed offer fails the test at once
// instead of feeding blank lines until the go test timeout.
const maxBlankAnswers = 200

// modelsInput scripts the answers that reach the models section by its own
// prompt instead of counting the earlier ones: it answers every prompt with
// a blank line until out shows the models offer, then answers "y" to it and
// feeds the given answers, one line per read, before reporting EOF. out must
// be the buffer the wizard writes to. The wizard's scanner reads one line at
// a time, so out always holds the prompt it is answering when Read runs.
func modelsInput(out *bytes.Buffer, answers ...string) io.Reader {
	queue := append([]string{"y"}, answers...)
	blanks := 0
	return readerFunc(func(p []byte) (int, error) {
		line := ""
		if !strings.Contains(out.String(), modelsOffer) {
			if blanks == maxBlankAnswers {
				return 0, fmt.Errorf("models offer %q never appeared after %d blank answers", modelsOffer, maxBlankAnswers)
			}
			blanks++
		} else {
			if len(queue) == 0 {
				return 0, io.EOF
			}
			line, queue = queue[0], queue[1:]
		}
		return copy(p, line+"\n"), nil
	})
}

func TestModelsInput_FailsWhenTheOfferNeverAppears(t *testing.T) {
	var out bytes.Buffer
	in := modelsInput(&out)
	buf := make([]byte, 64)
	for i := 0; i < maxBlankAnswers; i++ {
		if _, err := in.Read(buf); err != nil {
			t.Fatalf("read %d: unexpected error %v", i, err)
		}
	}
	if _, err := in.Read(buf); err == nil || !strings.Contains(err.Error(), "never appeared") {
		t.Fatalf("read past the bound: error = %v, want one naming the missing offer", err)
	}
}

// promptAnswer answers the first prompt whose text contains prompt.
type promptAnswer struct {
	prompt, answer string
}

// promptInput scripts answers by prompt text instead of by position: each
// read looks at the output written since the previous read, gives the next
// pending answer once its prompt appears there, and answers blank
// otherwise. It reports EOF once every answer was given, and fails after
// maxBlankAnswers blank answers in a row so a missing prompt cannot hang the
// test. out must be the buffer the wizard writes to.
func promptInput(out *bytes.Buffer, answers []promptAnswer) io.Reader {
	seen, blanks := 0, 0
	return readerFunc(func(p []byte) (int, error) {
		if len(answers) == 0 {
			return 0, io.EOF
		}
		fresh := out.String()[seen:]
		seen = out.Len()
		line := ""
		if strings.Contains(fresh, answers[0].prompt) {
			line, answers, blanks = answers[0].answer, answers[1:], 0
		} else {
			if blanks == maxBlankAnswers {
				return 0, fmt.Errorf("prompt %q never appeared after %d blank answers", answers[0].prompt, maxBlankAnswers)
			}
			blanks++
		}
		return copy(p, line+"\n"), nil
	})
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

var tableRowRe = regexp.MustCompile(`^\s*\d+\) ([a-z-]+) `)

// phaseLine returns the first output line after "Phases:" that contains needle.
func phaseLine(out, needle string) string {
	_, after, _ := strings.Cut(out, "Phases:")
	return lineWith(after, needle)
}

func lineWith(out, needle string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}

// The models table says what each role does and which gentle-ai phase it
// matches, and a legend explains the group shortcuts before the prompt.
func TestRun_ModelsSection_TableShowsPurposeEquivalentAndGroupLegend(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}
	opts := wizard.Options{Paths: paths, SkipSkills: true, SkipRepos: true, SkipCommands: true, NoRefresh: true}

	var out bytes.Buffer
	if _, err := wizard.Run(deps, modelsInput(&out, "done"), &out, opts); err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	got := out.String()

	header := lineWith(got, "ROLE ")
	for _, col := range []string{"WHAT IT DOES", "GENTLE-AI"} {
		if !strings.Contains(header, col) {
			t.Errorf("table header %q lacks column %q", header, col)
		}
	}
	if row := lineWith(got, ") kaji-security"); !strings.Contains(row, "audit pass: security") || !strings.Contains(row, "review-risk") {
		t.Errorf("kaji-security row = %q, want purpose and review-risk", row)
	}
	if row := lineWith(got, ") kaworu"); !strings.Contains(row, "writes the failing tests first") || strings.Contains(row, "sdd-") || !strings.HasSuffix(strings.TrimSpace(row), "-") {
		t.Errorf("kaworu row = %q, want purpose and a '-' equivalent", row)
	}
	if row := lineWith(got, ") fuyutsuki"); !strings.HasSuffix(strings.TrimSpace(row), "-") {
		t.Errorf("fuyutsuki row = %q, want a '-' equivalent", row)
	}
	// Every purpose starts under its header and is printed whole, whatever
	// the fixture's model and source widths are.
	purposeCol, equivalentCol := strings.Index(header, "WHAT IT DOES"), strings.Index(header, "GENTLE-AI")
	if purposeCol < 0 || equivalentCol <= purposeCol {
		t.Fatalf("table header %q: WHAT IT DOES at %d, GENTLE-AI at %d", header, purposeCol, equivalentCol)
	}
	info := config.Roles().Info
	checked := 0
	for _, l := range strings.Split(got, "\n") {
		m := tableRowRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		role, ok := info[m[1]]
		if !ok {
			continue // a numbered row of another menu
		}
		checked++
		purpose := role.Purpose
		if strings.Index(l, purpose) != purposeCol {
			t.Errorf("%s purpose not aligned under WHAT IT DOES (col %d): %q", m[1], purposeCol, l)
		}
		if len(l) < equivalentCol || strings.TrimSpace(l[purposeCol:equivalentCol]) != purpose {
			t.Errorf("%s purpose cut off or GENTLE-AI misaligned (col %d): %q", m[1], equivalentCol, l)
		}
	}
	if want := len(config.Roles().AllRoles); checked != want {
		t.Errorf("alignment checked %d table rows, want one per role (%d)", checked, want)
	}
	for group, desc := range config.Roles().GroupDescriptions {
		if !strings.Contains(got, group+" = "+desc) {
			t.Errorf("group legend lacks %q", group+" = "+desc)
		}
	}
	if legend, prompt := strings.Index(got, "magi = "), strings.Index(got, "Role (name, number"); legend < 0 || legend > prompt {
		t.Errorf("group legend must precede the role prompt (legend at %d, prompt at %d)", legend, prompt)
	}
}

// For a single role, the phase picker lists the role's gentle-ai equivalent
// first and marks it; picking it writes from:<phase>.
func TestRun_ModelsSection_PhasePickerListsEquivalentFirst(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	paths := testPaths(root, home)
	state := `{"claude_phase_assignments":{"jd-judge-a":{"model":"opus","effort":"high"},"jd-judge-b":{"model":"sonnet","effort":"medium"}}}`
	if err := os.MkdirAll(filepath.Dir(paths.State), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.State, []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}
	opts := wizard.Options{Paths: paths, SkipSkills: true, SkipRepos: true, SkipCommands: true, NoRefresh: true}

	var out bytes.Buffer
	in := modelsInput(&out, "casper", "7", "1", "", "done", "y")
	if _, err := wizard.Run(deps, in, &out, opts); err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}

	if first := phaseLine(out.String(), "  1) "); !strings.Contains(first, "jd-judge-a") || !strings.Contains(first, "(equivalent)") {
		t.Errorf("first phase = %q, want jd-judge-a marked (equivalent)", first)
	}
	if second := phaseLine(out.String(), "  2) "); !strings.Contains(second, "jd-judge-b") || strings.Contains(second, "(equivalent)") {
		t.Errorf("second phase = %q, want jd-judge-b unmarked", second)
	}
	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if want := "casper: { from: jd-judge-a }"; !strings.Contains(string(got), want) {
		t.Errorf("expected %q in models: block:\n%s", want, got)
	}
}

// A gentle-ai major other than 4 is warned about in the prerequisites
// block, and the warning names the required major.
func TestRun_Prerequisites_UnsupportedMajorWarnsAndNamesSupportedMajors(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "nerv.yaml"))
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"gentle-ai --version": {Stdout: "gentle-ai version 2.9.0\n"},
	}}
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathNone}
	opts := wizard.Options{Paths: testPaths(root, home), SkipSkills: true, SkipRepos: true, SkipCommands: true, SkipModels: true, NoRefresh: true}

	var out bytes.Buffer
	_, _ = wizard.Run(deps, strings.NewReader(""), &out, opts) // EOF aborts after the prerequisites block

	if line := lineWith(out.String(), "gentle-ai      :"); !strings.Contains(line, "2.9.0") || !strings.Contains(line, "WARNING: NERV requires gentle-ai 4.x") {
		t.Errorf("gentle-ai line = %q, want a warning naming gentle-ai 4.x; output:\n%s", line, out.String())
	}
}

// mutatingCalls filters out the informational "gentle-ai --version"
// prerequisites check (printPrerequisites runs it unconditionally, before
// any prompt, and it never mutates anything) so EOF-abort tests can
// assert on the calls the 2026-09-29 incident actually cared about:
// `claude plugin uninstall/install` and `npx skills add -g`.
func mutatingCalls(calls []envtest.Call) []envtest.Call {
	var out []envtest.Call
	for _, c := range calls {
		if c.Name == "gentle-ai" {
			continue
		}
		out = append(out, c)
	}
	return out
}

// diffLineCount counts differing lines between a and b (positionally, like
// tests/configure.test.ps1's own e2e diff assertions).
func diffLineCount(a, b string) int {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	max := len(aLines)
	if len(bLines) > max {
		max = len(bLines)
	}
	diff := 0
	for i := 0; i < max; i++ {
		var al, bl string
		if i < len(aLines) {
			al = aLines[i]
		}
		if i < len(bLines) {
			bl = bLines[i]
		}
		if al != bl {
			diff++
		}
	}
	return diff
}

// ---------------------------------------------------------------------------
// Legacy provider settings (github-projects and jira sub-blocks in
// tasks.providers, a tasks.provider: jira line), left by releases before their
// removal, are stripped by a wizard save; every other byte stays as written.
// ---------------------------------------------------------------------------

const legacyProviderLines = "" +
	"    # legacy stubs, kept by hand\n" +
	"    github-projects: { task_ref_prefix: gh, owner: \"\", project_number: 0 }    # later\n" +
	"    jira: { task_ref_prefix: jira, site: \"\", project_key: \"\" }               # later\n"

func legacyFixture(t *testing.T, provider string) string {
	t.Helper()
	const anchor = "artifacts:\n"
	fixture := strings.Replace(fixtureLF, anchor, legacyProviderLines+anchor, 1)
	if provider != "teamwork" {
		old := "  provider: teamwork                #"
		if !strings.Contains(fixture, old) {
			t.Fatal("provider line not found in the fixture")
		}
		fixture = strings.Replace(fixture, old, "  provider: "+provider+strings.Repeat(" ", 20-len(provider))+"#", 1)
	}
	return fixture
}

func TestRun_LegacyProviderSettings_AreStrippedOnSave(t *testing.T) {
	for _, provider := range []string{"teamwork", "jira"} {
		t.Run(provider, func(t *testing.T) {
			root := t.TempDir()
			home := t.TempDir()
			paths := testPaths(root, home)
			fixture := legacyFixture(t, provider)
			if err := os.WriteFile(paths.Config, []byte(fixture), 0o644); err != nil {
				t.Fatal(err)
			}
			deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}
			opts := skipAll()
			opts.Paths = paths

			var out bytes.Buffer
			in := promptInput(&out, []promptAnswer{
				{prompt: "Base branch", answer: "develop2"},
				{prompt: "Write to ", answer: "y"},
			})
			summary, err := wizard.Run(deps, in, &out, opts)
			if err != nil {
				t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
			}
			if !summary.Changed {
				t.Fatalf("Changed = false, want true; output:\n%s", out.String())
			}

			got, err := os.ReadFile(paths.Config)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), "base_branch: develop2") {
				t.Errorf("base_branch not updated:\n%s", got)
			}
			if strings.Contains(string(got), "github-projects") || strings.Contains(string(got), "jira: {") {
				t.Errorf("legacy provider blocks survived:\n%s", got)
			}

			// What the save must remove: the blocks, plus the provider line
			// when it still holds the removed provider.
			cleaned := strings.Replace(fixture, legacyProviderLines, "", 1)
			wantLine := "Removed legacy task provider settings: tasks.providers.github-projects, tasks.providers.jira"
			if provider == "jira" {
				cleaned = removeLineWith(cleaned, "  provider: jira")
				wantLine = "Removed legacy task provider settings: tasks.provider (jira), tasks.providers.github-projects, tasks.providers.jira"
			}
			if !strings.Contains(out.String(), wantLine) {
				t.Errorf("output missing %q:\n%s", wantLine, out.String())
			}
			if diff := diffLineCount(cleaned, string(got)); diff != 1 {
				t.Errorf("diff line count against the cleaned fixture = %d, want 1; got:\n%s", diff, got)
			}
			if provider == "jira" && strings.Contains(string(got), "  provider:") {
				t.Errorf("tasks.provider survived:\n%s", got)
			}
		})
	}
}

// removeLineWith drops the one line of s that contains marker.
func removeLineWith(s, marker string) string {
	lines := strings.SplitAfter(s, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if !strings.Contains(l, marker) {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "")
}

// The models section writes through the shared store too: saving an override
// into a legacy file strips the removed provider settings and says so.
func TestRun_ModelsSection_StripsLegacyProviderSettings(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	paths := testPaths(root, home)
	fixture := legacyFixture(t, "jira")
	if err := os.WriteFile(paths.Config, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}
	opts := wizard.Options{Paths: paths, SkipSkills: true, SkipRepos: true, SkipCommands: true, NoRefresh: true}

	var out bytes.Buffer
	in := modelsInput(&out, "magi", "2", "3", "done", "y")
	if _, err := wizard.Run(deps, in, &out, opts); err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}

	got, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "github-projects") || strings.Contains(string(got), "provider: jira") {
		t.Errorf("legacy settings survived the models write:\n%s", got)
	}
	if !strings.Contains(out.String(), "Removed legacy task provider settings: tasks.provider (jira), tasks.providers.github-projects, tasks.providers.jira") {
		t.Errorf("output does not report the removal:\n%s", out.String())
	}
}
