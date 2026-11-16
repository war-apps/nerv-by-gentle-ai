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

	if err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "", false); err != nil {
		t.Fatalf("offerEcosystemTools() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "--tools") || !strings.Contains(stdout.String(), "nerv-task") {
		t.Errorf("expected a --tools hint naming nerv-task, got: %q", stdout.String())
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

	if err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "", true); err != nil {
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

	if err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "nerv-task", false); err != nil {
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

	err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "not-a-tool", false)
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

	if err := offerEcosystemTools(context.Background(), &stdout, opts, home, "upgrade", "nerv-task", false); err != nil {
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
	opts.EcosystemAPIBase = map[string]string{"nerv-task": srv.URL}

	if err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "", false); err != nil {
		t.Fatalf("offerEcosystemTools() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "not installed") {
		t.Errorf("expected the state line to show 'not installed', got: %q", stdout.String())
	}
	if len(run.Calls) != 1 || run.Calls[0].Name != "sh" {
		t.Fatalf("Calls = %+v, want exactly one sh call after confirming yes", run.Calls)
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

	if err := offerEcosystemTools(context.Background(), &stdout, opts, home, "install", "", false); err != nil {
		t.Fatalf("offerEcosystemTools() error = %v", err)
	}
	if len(run.Calls) != 0 {
		t.Errorf("Calls = %v, want none (confirmed no)", run.Calls)
	}
}

type notFoundForTest struct{}

func (notFoundForTest) Error() string { return "executable file not found in $PATH" }

var errNotFoundForTest error = notFoundForTest{}
