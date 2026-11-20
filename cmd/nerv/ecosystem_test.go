package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

func TestOfferEcosystemTools_NotATerminal_NoFlags_PrintsHintOnly(t *testing.T) {
	var stdout bytes.Buffer
	home := t.TempDir()
	opts := testOptions(home)
	run := &runnertest.FakeRunner{}
	opts.Runner = run

	if _, err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "", false); err != nil {
		t.Fatalf("offerEcosystemTools() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "--tools") || !strings.Contains(stdout.String(), "nerv-task, nerv-release") {
		t.Errorf("expected a --tools hint naming nerv-task and nerv-release, got: %q", stdout.String())
	}
	if len(run.Calls) != 0 {
		t.Errorf("Calls = %v, want none (no flags, no terminal)", run.Calls)
	}
}

func TestOfferEcosystemTools_NoTools_SkipsEntirely(t *testing.T) {
	var stdout bytes.Buffer
	home := t.TempDir()
	opts := testOptions(home)
	opts.IsTerminal = func() bool { return true }
	run := &runnertest.FakeRunner{}
	opts.Runner = run

	if _, err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "", true); err != nil {
		t.Fatalf("offerEcosystemTools() error = %v", err)
	}
	if len(run.Calls) != 0 {
		t.Errorf("Calls = %v, want none (--no-tools)", run.Calls)
	}
}

func TestOfferEcosystemTools_ExplicitToolsRunsInstallScript(t *testing.T) {
	var stdout bytes.Buffer
	home := t.TempDir()
	opts := testOptions(home)
	run := &runnertest.FakeRunner{}
	opts.Runner = run
	opts.GOOS = "linux"

	if _, err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "nerv-task", false); err != nil {
		t.Fatalf("offerEcosystemTools() error = %v", err)
	}
	if len(run.Calls) != 1 || run.Calls[0].Name != "sh" {
		t.Fatalf("Calls = %+v, want exactly one sh call", run.Calls)
	}
	joined := strings.Join(run.Calls[0].Args, " ")
	if !strings.Contains(joined, "curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-task-by-gentle-ai/main/scripts/install.sh | sh") {
		t.Errorf("args = %v, want the nerv-task install.sh one-liner", run.Calls[0].Args)
	}
}

func TestOfferEcosystemTools_ExplicitUnknownTool_ReturnsRefusal(t *testing.T) {
	var stdout bytes.Buffer
	home := t.TempDir()
	opts := testOptions(home)

	_, err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "not-a-tool", false)
	if err == nil {
		t.Fatal("offerEcosystemTools() error = nil, want a refusal for an unknown tool name")
	}
}

func TestOfferEcosystemTools_UpgradeActionRunsToolUpgrade(t *testing.T) {
	var stdout bytes.Buffer
	home := t.TempDir()
	opts := testOptions(home)
	run := &runnertest.FakeRunner{}
	opts.Runner = run

	if _, err := offerEcosystemTools(context.Background(), &stdout, opts, home, "upgrade", "nerv-task", false); err != nil {
		t.Fatalf("offerEcosystemTools() error = %v", err)
	}
	if len(run.Calls) != 1 || run.Calls[0].Name != "nerv-task" || len(run.Calls[0].Args) != 1 || run.Calls[0].Args[0] != "upgrade" {
		t.Fatalf("Calls = %+v, want one nerv-task upgrade call (PATH-resolved, no detected state)", run.Calls)
	}
}

func TestOfferEcosystemTools_Interactive_ConfirmedYes_ShowsStateAndInstalls(t *testing.T) {
	var stdout bytes.Buffer
	home := t.TempDir()
	opts := testOptions(home)
	opts.IsTerminal = func() bool { return true }
	opts.Stdin = strings.NewReader("y\n")
	opts.LookPath = func(string) (string, error) { return "", errNotFoundForTest }
	run := &runnertest.FakeRunner{}
	opts.Runner = run
	opts.GOOS = "linux"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v1.1.0"}`))
	}))
	defer srv.Close()
	opts.HTTPClient = srv.Client()
	opts.EcosystemAPIBase = map[string]string{"nerv-task": srv.URL, "nerv-release": srv.URL}

	if _, err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "", false); err != nil {
		t.Fatalf("offerEcosystemTools() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "not installed") {
		t.Errorf("expected the state line to show 'not installed', got: %q", stdout.String())
	}
	if len(run.Calls) != 1 || run.Calls[0].Name != "sh" {
		t.Fatalf("Calls = %+v, want exactly one sh call after confirming yes", run.Calls)
	}
}

// S1: the interactive offer lists nerv-release after nerv-task, each with
// its state, and asks each one in that order.
func TestOfferEcosystemTools_Interactive_ListsBothToolsWithState(t *testing.T) {
	var stdout bytes.Buffer
	home := t.TempDir()
	opts := testOptions(home)
	opts.IsTerminal = func() bool { return true }
	opts.Stdin = strings.NewReader("n\ny\n")
	opts.GOOS = "linux"
	// nerv-task is on PATH at 1.0.0; nerv-release is not installed.
	opts.LookPath = func(name string) (string, error) {
		if name == "nerv-task" {
			return "/usr/bin/nerv-task", nil
		}
		return "", errNotFoundForTest
	}
	run := &runnertest.FakeRunner{Responses: map[string]runnertest.Response{
		"/usr/bin/nerv-task version --json": {Stdout: `{"binary":"1.0.0"}`},
	}}
	opts.Runner = run

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v1.1.0"}`))
	}))
	defer srv.Close()
	opts.HTTPClient = srv.Client()
	opts.EcosystemAPIBase = map[string]string{"nerv-task": srv.URL, "nerv-release": srv.URL}

	selected, err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "", false)
	if err != nil {
		t.Fatalf("offerEcosystemTools() error = %v", err)
	}
	out := stdout.String()
	ti, ri := strings.Index(out, "nerv-task: "), strings.Index(out, "nerv-release: not installed")
	if ti < 0 || ri < 0 || ti > ri {
		t.Errorf("want a nerv-task state line, then 'nerv-release: not installed':\n%s", out)
	}
	if !strings.Contains(out, "Install nerv-task? [y/N]: Install nerv-release? [y/N]: ") {
		t.Errorf("want nerv-task asked, then nerv-release:\n%s", out)
	}
	if len(selected) != 1 || selected[0].Name != "nerv-release" {
		t.Fatalf("selected = %+v, want nerv-release only", selected)
	}
	var shCalls []string
	for _, c := range run.Calls {
		if c.Name == "sh" {
			shCalls = append(shCalls, strings.Join(c.Args, " "))
		}
	}
	if len(shCalls) != 1 || !strings.Contains(shCalls[0], "war-apps/nerv-release-by-gentle-ai/main/scripts/install.sh") {
		t.Errorf("sh calls = %v, want one nerv-release install.sh run", shCalls)
	}
}

// S1: without a terminal, --tools nerv-release runs nerv-release's
// official install script (install.ps1 on Windows), and nothing else.
func TestOfferEcosystemTools_ExplicitNervReleaseRunsItsInstallScript(t *testing.T) {
	tests := []struct {
		goos, name, want string
	}{
		{"linux", "sh", "curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-release-by-gentle-ai/main/scripts/install.sh | sh"},
		{"windows", "powershell", "https://raw.githubusercontent.com/war-apps/nerv-release-by-gentle-ai/main/scripts/install.ps1"},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			var stdout bytes.Buffer
			home := t.TempDir()
			opts := testOptions(home)
			run := &runnertest.FakeRunner{}
			opts.Runner = run
			opts.GOOS = tt.goos

			if _, err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "nerv-release", false); err != nil {
				t.Fatalf("offerEcosystemTools() error = %v", err)
			}
			if len(run.Calls) != 1 || run.Calls[0].Name != tt.name {
				t.Fatalf("Calls = %+v, want exactly one %s call", run.Calls, tt.name)
			}
			if joined := strings.Join(run.Calls[0].Args, " "); !strings.Contains(joined, tt.want) {
				t.Errorf("args = %v, want them to contain %q", run.Calls[0].Args, tt.want)
			}
		})
	}
}

func TestOfferEcosystemTools_Interactive_ConfirmedNo_InstallsNothing(t *testing.T) {
	var stdout bytes.Buffer
	home := t.TempDir()
	opts := testOptions(home)
	opts.IsTerminal = func() bool { return true }
	opts.Stdin = strings.NewReader("n\n")
	opts.LookPath = func(string) (string, error) { return "", errNotFoundForTest }
	run := &runnertest.FakeRunner{}
	opts.Runner = run

	if _, err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "", false); err != nil {
		t.Fatalf("offerEcosystemTools() error = %v", err)
	}
	if len(run.Calls) != 0 {
		t.Errorf("Calls = %v, want none (confirmed no)", run.Calls)
	}
}

type notFoundForTest struct{}

func (notFoundForTest) Error() string { return "executable file not found in $PATH" }

var errNotFoundForTest error = notFoundForTest{}
