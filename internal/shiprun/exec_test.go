package shiprun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// Real-process tests: each needs its interpreter on PATH and skips without
// it (on Windows, bash must be Git Bash).

func needShell(t *testing.T, shell string) {
	t.Helper()
	if testing.Short() {
		t.Skip("real processes skipped in -short mode")
	}
	if _, err := (&Runner{}).interpreter(shell); err != nil {
		t.Skipf("%s not available: %v", shell, err)
	}
}

// realRepo returns a repository root holding files (path -> content).
func realRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// runReal runs one command for project "demo" and returns its streamed
// output and error.
func runReal(t *testing.T, root string, step string, cmd shipconfig.Command, vars Vars) (string, error) {
	t.Helper()
	var out bytes.Buffer
	if vars.Project == "" {
		vars.Project = "demo"
	}
	err := (&Runner{RepoRoot: root, Output: &out}).Run(context.Background(), Step{Name: step, Command: cmd}, vars)
	return out.String(), err
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	var se *StepError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want a *StepError", err)
	}
	return se.ExitCode
}

func TestReal_BashInline(t *testing.T) {
	needShell(t, "bash")
	root := realRepo(t, nil)
	out, err := runReal(t, root, "build", shipconfig.Command{Run: "echo hello; echo oops >&2", Shell: "bash"}, Vars{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "[demo:build] hello\n[demo:build] oops\n" {
		t.Errorf("output %q", out)
	}

	for _, run := range []string{"false | true; echo after", "false; echo after", "exit 4"} {
		out, err := runReal(t, root, "build", shipconfig.Command{Run: run, Shell: "bash"}, Vars{})
		if err == nil || strings.Contains(out, "after") {
			t.Errorf("%q: err %v, output %q; want a failure before echo", run, err, out)
		}
	}
	_, err = runReal(t, root, "build", shipconfig.Command{Run: "exit 4", Shell: "bash"}, Vars{})
	if code := exitCode(t, err); code != 4 {
		t.Errorf("exit code %d, want 4", code)
	}
}

func TestReal_BashFile(t *testing.T) {
	needShell(t, "bash")
	root := realRepo(t, map[string]string{
		"scripts/ok.sh":   "false | true\necho \"file ran in $(basename \"$PWD\")\"\n",
		"scripts/fail.sh": "echo before\nexit 7\n",
		"sub/.keep":       "",
	})
	out, err := runReal(t, root, "prepare", shipconfig.Command{Run: "scripts/ok.sh", Shell: "bash", Workdir: "sub"}, Vars{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "[demo:prepare] file ran in sub\n" {
		t.Errorf("output %q", out)
	}
	out, err = runReal(t, root, "prepare", shipconfig.Command{Run: "scripts/fail.sh", Shell: "bash"}, Vars{})
	if code := exitCode(t, err); code != 7 || !strings.Contains(err.Error(), "before") || out != "[demo:prepare] before\n" {
		t.Errorf("exit %d, err %v, output %q", code, err, out)
	}
}

// Before PowerShell 7.3 a failing native command does not stop an inline
// script, so the `exit 0` epilogue would turn it into a success: the prelude
// refuses to run on an older pwsh. The installed pwsh is newer, so the test
// raises the minimum instead.
func TestReal_PwshInlineRefusesAnOlderPowerShell(t *testing.T) {
	needShell(t, "pwsh")
	saved := pwshPrelude
	pwshPrelude = buildPwshPrelude(99, 0)
	t.Cleanup(func() { pwshPrelude = saved })

	root := realRepo(t, nil)
	out, err := runReal(t, root, "build", shipconfig.Command{Run: `Write-Output after`, Shell: "pwsh"}, Vars{})
	if err == nil || exitCode(t, err) != pwshTooOldExit || strings.Contains(out, "] after\n") || !strings.Contains(err.Error(), "PowerShell 99.0 or newer") {
		t.Fatalf("err %v, output %q; want exit %d naming the minimum before running the command", err, out, pwshTooOldExit)
	}
}

func TestReal_PwshInlineFailuresExitNonZero(t *testing.T) {
	needShell(t, "pwsh")
	root := realRepo(t, nil)
	for _, run := range []string{
		`throw "boom"; Write-Output after`,
		`Get-Item ./does-not-exist; Write-Output after`,
		`git rev-parse --verify refs/heads/nerv-does-not-exist; Write-Output after`,
		`exit 5`,
	} {
		out, err := runReal(t, root, "lint", shipconfig.Command{Run: run, Shell: "pwsh"}, Vars{})
		// The error view quotes the source line, so look for the output line.
		if err == nil || exitCode(t, err) == 0 || strings.Contains(out, "] after\n") {
			t.Errorf("%q: err %v, output %q; want a non-zero exit before Write-Output", run, err, out)
		}
		if strings.Contains(err.Error(), "\x1b[") {
			t.Errorf("%q: the error carries terminal escape sequences: %q", run, err)
		}
	}
	out, err := runReal(t, root, "lint", shipconfig.Command{Run: `Write-Output "v=$env:NERV_RELEASE_VERSION"`, Shell: "pwsh"}, Vars{Version: "2.0.0"})
	if err != nil || out != "[demo:lint] v=2.0.0\n" {
		t.Errorf("err %v, output %q", err, out)
	}
}

func TestReal_PwshFile(t *testing.T) {
	needShell(t, "pwsh")
	root := realRepo(t, map[string]string{
		"scripts/bump.ps1": "Write-Output \"bump $env:NERV_RELEASE_TAG\"\nexit 3\n",
	})
	out, err := runReal(t, root, "prepare", shipconfig.Command{Run: "scripts/bump.ps1", Shell: "pwsh"}, Vars{Tag: "v1.0.0"})
	if code := exitCode(t, err); code != 3 || out != "[demo:prepare] bump v1.0.0\n" {
		t.Errorf("exit %d, output %q", code, out)
	}
}

func TestReal_Node(t *testing.T) {
	needShell(t, "node")
	root := realRepo(t, map[string]string{
		"scripts/notes.mjs": "console.log(`notes ${process.env.NERV_RELEASE_PROJECT} ${process.env.EXTRA}`)\n",
		"scripts/fail.cjs":  "console.error('bad'); process.exit(9)\n",
	})
	out, err := runReal(t, root, "success", shipconfig.Command{Run: "console.log(process.env.NERV_RELEASE_DRY_RUN)", Shell: "node"}, Vars{DryRun: true})
	if err != nil || out != "[demo:success] true\n" {
		t.Errorf("node -e: err %v, output %q", err, out)
	}
	out, err = runReal(t, root, "success", shipconfig.Command{Run: "scripts/notes.mjs", Shell: "node", Env: map[string]string{"EXTRA": "x"}}, Vars{})
	if err != nil || out != "[demo:success] notes demo x\n" {
		t.Errorf("node file: err %v, output %q", err, out)
	}
	_, err = runReal(t, root, "success", shipconfig.Command{Run: "scripts/fail.cjs", Shell: "node"}, Vars{})
	if code := exitCode(t, err); code != 9 || !strings.Contains(err.Error(), "bad") {
		t.Errorf("exit %d, err %v", code, err)
	}
}

func TestReal_EnvironmentAndWorkdir(t *testing.T) {
	needShell(t, "bash")
	root := realRepo(t, map[string]string{"pkg/web/.keep": ""})
	vars := Vars{
		ProjectPath: "pkg/web", Version: "1.1.0", Channel: "beta", Tag: "web-v1.1.0", GitHead: "deadbeef",
		LastVersion: "1.0.0", NotesFile: "notes.md", Branch: "main", DryRun: false,
	}
	cmd := shipconfig.Command{Shell: "bash", Workdir: "pkg/web", Env: map[string]string{"HOOK_VAR": "hv"}, Run: `basename "$PWD"
for v in PROJECT PROJECT_PATH STAGE VERSION CHANNEL TAG GIT_HEAD LAST_VERSION NOTES_FILE BRANCH DRY_RUN; do
  n="NERV_RELEASE_$v"; echo "$v=${!n}"
done
echo "HOOK_VAR=$HOOK_VAR"
test "$NERV_RELEASE_REPO_ROOT" -ef "$PWD/../.." && echo root-ok`}
	out, err := runReal(t, root, "after_test", cmd, vars)
	if err != nil {
		t.Fatal(err)
	}
	want := ""
	for _, l := range []string{"web", "PROJECT=demo", "PROJECT_PATH=pkg/web", "STAGE=after_test", "VERSION=1.1.0", "CHANNEL=beta",
		"TAG=web-v1.1.0", "GIT_HEAD=deadbeef", "LAST_VERSION=1.0.0", "NOTES_FILE=notes.md", "BRANCH=main", "DRY_RUN=false",
		"HOOK_VAR=hv", "root-ok"} {
		want += "[demo:after_test] " + l + "\n"
	}
	if out != want {
		t.Errorf("output:\n%s\nwant:\n%s", out, want)
	}
}

// stdout and stderr lines reach the writer in the order the script wrote
// them, each with the prefix.
func TestReal_StreamingOrderWithPrefixes(t *testing.T) {
	needShell(t, "bash")
	var script, want strings.Builder
	for i := range 200 {
		redirect := ""
		if i%3 == 0 {
			redirect = " >&2"
		}
		fmt.Fprintf(&script, "echo line%d%s\n", i, redirect)
		fmt.Fprintf(&want, "[demo:test] line%d\n", i)
	}
	out, err := runReal(t, realRepo(t, nil), "test", shipconfig.Command{Run: script.String(), Shell: "bash"}, Vars{})
	if err != nil {
		t.Fatal(err)
	}
	if out != want.String() {
		t.Errorf("output out of order:\n%s", out)
	}
}

func TestReal_TimeoutStopsTheScript(t *testing.T) {
	needShell(t, "bash")
	start := time.Now()
	_, err := runReal(t, realRepo(t, nil), "test", shipconfig.Command{Run: "echo started; sleep 60", Shell: "bash", Timeout: "500ms"}, Vars{})
	if !errors.Is(err, ErrTimeout) || !strings.Contains(err.Error(), "started") {
		t.Errorf("err = %v, want a timeout carrying the output", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("returned after %s", elapsed)
	}
}

func TestReal_MissingInterpreterOnPath(t *testing.T) {
	if testing.Short() {
		t.Skip("real processes skipped in -short mode")
	}
	t.Setenv("PATH", t.TempDir())
	_, err := runReal(t, realRepo(t, nil), "test", shipconfig.Command{Run: "1", Shell: "node"}, Vars{})
	if !errors.Is(err, ErrInterpreterNotFound) {
		t.Errorf("err = %v, want ErrInterpreterNotFound", err)
	}
}

// An inline pwsh command's exit code says how the script ended: 0 when it
// completed (a native failure it handled does not count), 1 for an
// uncaught error, N for exit N.
func TestReal_PwshExitCodeFollowsHowTheScriptEnded(t *testing.T) {
	needShell(t, "pwsh")
	root := realRepo(t, nil)
	native := "git rev-parse --verify --quiet refs/heads/nerv-does-not-exist"
	tests := []struct {
		name string
		run  string
		want int
	}{
		{"handled with try/catch", "try { " + native + " } catch { Write-Output handled }", 0},
		{"checked and continued", "$PSNativeCommandUseErrorActionPreference = $false; " + native + "; if ($LASTEXITCODE -ne 0) { Write-Output checked }", 0},
		{"later native success", "try { " + native + " } catch { }; git --version", 0},
		{"unhandled native failure", native, 1},
		{"explicit exit", "exit 5", 5},
		{"throw", "throw 'boom'", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runReal(t, root, "lint", shipconfig.Command{Run: tt.run, Shell: "pwsh"}, Vars{})
			got := 0
			if err != nil {
				got = exitCode(t, err)
			}
			if got != tt.want {
				t.Errorf("exit code %d (err %v), want %d", got, err, tt.want)
			}
		})
	}
}

// failingWriter accepts n writes, then fails every later one.
type failingWriter struct {
	n      int
	writes []string
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if len(w.writes) >= w.n {
		return 0, errors.New("output closed")
	}
	w.writes = append(w.writes, string(p))
	return len(p), nil
}

// A failing output writer neither blocks nor kills the script: it runs to
// the end, later lines are dropped, and the write error is reported after.
func TestReal_OutputWriteErrorIsReportedAfterTheScript(t *testing.T) {
	needShell(t, "bash")
	root := realRepo(t, nil)
	w := &failingWriter{n: 1}
	run := `for i in $(seq 1 2000); do echo line$i; done; echo done > finished`
	err := (&Runner{RepoRoot: root, Output: w}).Run(context.Background(), Step{Name: "test", Command: shipconfig.Command{Run: run, Shell: "bash"}}, Vars{Project: "demo"})
	var se *StepError
	if !errors.As(err, &se) || !strings.Contains(err.Error(), "writing its output") || !strings.Contains(err.Error(), "output closed") {
		t.Fatalf("err = %v, want a StepError about the output", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "finished")); statErr != nil {
		t.Errorf("the script did not run to the end: %v", statErr)
	}
	if len(w.writes) != 1 || w.writes[0] != "[demo:test] line1\n" {
		t.Errorf("writes = %q, want only the first line", w.writes)
	}
}

func TestReal_CaptureReturnsStdoutAndStreamsStderr(t *testing.T) {
	needShell(t, "bash")
	root := realRepo(t, nil)
	var out bytes.Buffer
	r := &Runner{RepoRoot: root, Output: &out}
	got, err := r.Capture(context.Background(), Step{Name: "chrome", Command: shipconfig.Command{Run: "echo 1.2.3; echo resolving >&2", Shell: "bash"}}, Vars{Project: "pipeline"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.2.3\n" {
		t.Errorf("stdout %q, want only the command's stdout", got)
	}
	if out.String() != "[pipeline:chrome] resolving\n" {
		t.Errorf("streamed %q, want stderr prefixed", out.String())
	}
	_, err = r.Capture(context.Background(), Step{Name: "chrome", Command: shipconfig.Command{Run: "echo partial; exit 3", Shell: "bash"}}, Vars{Project: "pipeline"})
	if code := exitCode(t, err); code != 3 {
		t.Errorf("exit code %d", code)
	}
}
