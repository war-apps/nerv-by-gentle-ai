package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-gentle-ai"
	"github.com/war-apps/nerv-gentle-ai/internal/version"
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

func mustPluginFS(t *testing.T) fs.FS {
	t.Helper()
	return nerv.PluginFS()
}
