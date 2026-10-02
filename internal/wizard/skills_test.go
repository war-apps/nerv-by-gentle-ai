package wizard_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/wizard"
)

// runSkillsSection drives the wizard through the required-skills section only
// (every later section skipped, the 28 user-config prompts kept blank) with the
// given scripted answers for the skills prompts, against an empty skills
// directory so every gentle-ai skill is missing.
func runSkillsSection(t *testing.T, runner *envtest.FakeRunner, answers ...string) string {
	t.Helper()
	root, home := t.TempDir(), t.TempDir()
	writeFixture(t, testPaths(root, home).Config)
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), Runner: runner, Now: fixedNow, LookPath: lookPathNone}
	opts := wizard.Options{Paths: testPaths(root, home), SkipModels: true, SkipRepos: true, SkipCommands: true, NoRefresh: true}

	in := strings.NewReader(strings.Join(answers, "\n") + "\n" + strings.Repeat("\n", 28))
	var out bytes.Buffer
	if _, err := wizard.Run(deps, in, &out, opts); err != nil && !errors.Is(err, wizard.ErrInputClosed) {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	return out.String()
}

func syncCalls(runner *envtest.FakeRunner) int {
	n := 0
	for _, c := range runner.Calls {
		if c.Name == "gentle-ai" && len(c.Args) > 0 && c.Args[0] == "sync" {
			n++
		}
	}
	return n
}

func TestRun_SkillsSection_ConfirmYes_RunsSyncAndPrintsItsLine(t *testing.T) {
	runner := &envtest.FakeRunner{}

	out := runSkillsSection(t, runner, "y", "y")

	if n := syncCalls(runner); n != 1 {
		t.Fatalf("sync ran %d times, want once; output:\n%s", n, out)
	}
	for _, want := range []string{"managed files", "~/.claude/CLAUDE.md", "-> gentle-ai sync --agents claude-code --skills "} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestRun_SkillsSection_ConfirmYes_FailedSync_PrintsWarningWithDiagnostic(t *testing.T) {
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{"gentle-ai": {ExitCode: 1, Stderr: "boom: no such agent\n"}}}

	out := runSkillsSection(t, runner, "y", "y")

	warning := "Warning: gentle-ai sync failed; the gentle-ai skills may still be missing.\n  boom: no such agent\n"
	if !strings.Contains(out, warning) {
		t.Errorf("output lacks the warning with its diagnostic:\n%s", out)
	}
	if !strings.Contains(out, "Remedy: run 'gentle-ai install'") {
		t.Errorf("output lacks the remedies after the failed sync:\n%s", out)
	}
}

func TestRun_SkillsSection_ConfirmDefaultNo_SkipsSyncKeepsRemedies(t *testing.T) {
	for name, answer := range map[string]string{"blank": "", "no": "n"} {
		t.Run(name, func(t *testing.T) {
			runner := &envtest.FakeRunner{}

			out := runSkillsSection(t, runner, "y", answer)

			if n := syncCalls(runner); n != 0 {
				t.Fatalf("sync ran %d times, want 0; output:\n%s", n, out)
			}
			if !strings.Contains(out, "gentle-ai sync skipped") || !strings.Contains(out, "Remedy: run 'gentle-ai install'") {
				t.Errorf("output lacks the skipped line or the remedies:\n%s", out)
			}
		})
	}
}
