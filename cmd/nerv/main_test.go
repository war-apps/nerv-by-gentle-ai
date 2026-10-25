package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
)

func TestRun_VersionPrintsBinaryAndPlugin(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"version"}, &stdout, &stderr, testOptions(t.TempDir()))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	pluginVersion, err := version.PluginVersion(mustPluginFS(t))
	if err != nil {
		t.Fatalf("reading plugin version: %v", err)
	}
	want := fmt.Sprintf("nerv %s (plugin %s)\n", version.Binary, pluginVersion)
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRun_VersionJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"version", "--json"}, &stdout, &stderr, testOptions(t.TempDir()))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	var got struct {
		Binary string `json:"binary"`
		Plugin string `json:"plugin"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (stdout=%q)", err, stdout.String())
	}
	if got.Binary != version.Binary {
		t.Errorf("json.binary = %q, want %q", got.Binary, version.Binary)
	}
	pluginVersion, err := version.PluginVersion(mustPluginFS(t))
	if err != nil {
		t.Fatalf("reading plugin version: %v", err)
	}
	if got.Plugin != pluginVersion {
		t.Errorf("json.plugin = %q, want %q", got.Plugin, pluginVersion)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRun_UnknownCommandPrintsUsageToStderrAndExits2(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"bogus"}, &stdout, &stderr, testOptions(t.TempDir()))

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage") && !strings.Contains(stderr.String(), "Usage") {
		t.Errorf("stderr = %q, want it to mention usage", stderr.String())
	}
}

func TestRun_NoArgsPrintsUsageAndExits0(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{}, &stdout, &stderr, testOptions(t.TempDir()))

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stdout.Len() == 0 {
		t.Error("expected usage text on stdout")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

// The usage lists nerv upgrade, and the upgrade options (an HTTP client, a
// releases API, an executable) change nothing for the other commands:
// nerv version still prints the same line and never reaches the network
// or runs a binary.
func TestRun_UsageListsUpgradeAndUpgradeOptionsLeaveVersionAlone(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{}, &stdout, &stderr, testOptions(t.TempDir())); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "\n  upgrade [--alpha | --rc]") {
		t.Errorf("usage lacks nerv upgrade:\n%s", stdout.String())
	}

	var plain, withUpgrade bytes.Buffer
	if code := run([]string{"version"}, &plain, &stderr, testOptions(t.TempDir())); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	opts := testOptions(t.TempDir())
	opts.ReleaseAPIBase = "http://127.0.0.1:1/unreachable"
	opts.ReleaseDownloadBase = "http://127.0.0.1:1/unreachable"
	opts.Executable = func() (string, error) { t.Error("version resolved the executable"); return "", nil }
	opts.RunBinary = func(context.Context, string, []string, io.Writer, io.Writer) (int, error) {
		t.Error("version ran a binary")
		return 0, nil
	}
	if code := run([]string{"version"}, &withUpgrade, &stderr, opts); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if withUpgrade.String() != plain.String() || stderr.Len() != 0 {
		t.Errorf("version = %q (stderr %q), want %q", withUpgrade.String(), stderr.String(), plain.String())
	}
}

func mustPluginFS(t *testing.T) fs.FS {
	t.Helper()
	return nerv.PluginFS()
}
