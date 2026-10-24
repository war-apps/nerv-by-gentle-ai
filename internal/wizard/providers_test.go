package wizard_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/wizard"
)

// writeSettings writes a settings.json enabling ids and returns its path.
func writeSettings(t *testing.T, home string, ids ...string) string {
	t.Helper()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	entries := make([]string, len(ids))
	for i, id := range ids {
		entries[i] = `"` + id + `":true`
	}
	if err := os.WriteFile(path, []byte(`{"enabledPlugins":{`+strings.Join(entries, ",")+`}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func scalarOf(t *testing.T, path, key string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return config.Parse(string(data)).Scalar(key)
}

// S3: without task@nerv enabled, the wizard shows no Task providers section,
// even when a provider plugin is enabled and its manifest is embedded.
func TestRun_TaskProviders_HiddenWithoutTheTaskPlugin(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "config.yml"))
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), ExtensionsFS: nerv.ExtensionsFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}
	opts := skipAll()
	opts.Paths = testPaths(root, home)
	opts.SettingsPath = writeSettings(t, home, "nerv@nerv", "task-teamwork@nerv")

	var out bytes.Buffer
	if _, err := wizard.Run(deps, strings.NewReader(strings.Repeat("\n", 12)), &out, opts); err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "Task providers") {
		t.Errorf("Task providers shown without task@nerv:\n%s", out.String())
	}
}

// S3: with task@nerv and one provider enabled, every user-scope scalar key
// of its manifest is prompted with its Prompt and the current value (or the
// Default), invalid values are asked again, and the answers are written to
// tasks.providers.<name>.<key>, keeping the rest of the file; tasks.provider
// is not asked for a single provider.
func TestRun_TaskProviders_PromptsTheEnabledProviderUserKeys(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "config.yml"))
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), ExtensionsFS: nerv.ExtensionsFS(), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}
	opts := skipAll()
	opts.Paths = testPaths(root, home)
	opts.SettingsPath = writeSettings(t, home, "nerv@nerv", "task@nerv", "task-teamwork@nerv")

	var out bytes.Buffer
	in := promptInput(&out, []promptAnswer{
		{prompt: "Your Teamwork user id (assigned on start and take, matched by /task:me) [686035]: ", answer: ""},
		{prompt: "Default Teamwork project id for new tasks and /task:all [1271726]: ", answer: "abc"},
		{prompt: "Invalid input. Expected int.", answer: "42"},
		{prompt: "Round every logged block up to this many minutes [15]: ", answer: "30"},
		{prompt: "Log time as billable (true|false) [false]: ", answer: "true"},
		{prompt: "Write to ", answer: "y"},
	})
	summary, err := wizard.Run(deps, in, &out, opts)
	if err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if !summary.Changed {
		t.Errorf("Changed = false, want true; output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "--- Task providers ---") {
		t.Errorf("no Task providers section:\n%s", out.String())
	}
	for _, repoKey := range []string{"Teamwork project id of this repository", "Task provider ("} {
		if strings.Contains(out.String(), repoKey) {
			t.Errorf("user flow asked %q:\n%s", repoKey, out.String())
		}
	}

	for key, want := range map[string]string{
		"tasks.providers.teamwork.assignee_id":         "686035",
		"tasks.providers.teamwork.default_project_id":  "42",
		"tasks.providers.teamwork.default_tasklist_id": "3951970",
		"tasks.providers.teamwork.rounding_minutes":    "30",
		"tasks.providers.teamwork.billable":            "true",
		"tasks.provider":                               "teamwork",
	} {
		if got, _ := scalarOf(t, opts.Paths.Config, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	const stages = "      stages: { inDev: DESARROLLO, testing: TESTING, implemented: IMPLEMENTA, blocked: BLOQUEA, canceled: CANCEL, pending: PENDIENTE, analysis: ANALISIS }\n"
	want := strings.Replace(fixtureLF, "default_project_id: 1271726", "default_project_id: 42", 1)
	want = strings.Replace(want, stages, stages+"      rounding_minutes: 30\n      billable: true\n", 1)
	if got, _ := os.ReadFile(opts.Paths.Config); string(got) != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
}

// twoProviders is the embedded extensions plus a second provider, a copy of
// the embedded one under another name.
func twoProviders(t *testing.T) fs.FS {
	t.Helper()
	ext := nerv.ExtensionsFS()
	mapFS := fstest.MapFS{}
	if err := fs.WalkDir(ext, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(ext, path)
		if err != nil {
			return err
		}
		mapFS[path] = &fstest.MapFile{Data: data}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(string(mapFS["task-teamwork/provider.yaml"].Data), "name: teamwork", "name: other", 1)
	mapFS["task-other/provider.yaml"] = &fstest.MapFile{Data: []byte(manifest)}
	mapFS["task-other/.claude-plugin/plugin.json"] = &fstest.MapFile{Data: []byte(`{"name":"task-other","version":"1.0.0","description":"other"}`)}
	for path, file := range mapFS {
		if rest, ok := strings.CutPrefix(path, "task-teamwork/procedures/"); ok {
			mapFS["task-other/procedures/"+rest] = file
		}
	}
	return mapFS
}

// S3: with more than one provider enabled, tasks.provider is asked among the
// enabled providers' names.
func TestRun_TaskProviders_AsksTheProviderWhenSeveralAreEnabled(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "config.yml"))
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), ExtensionsFS: twoProviders(t), Runner: noRunner(), Now: fixedNow, LookPath: lookPathNone}
	opts := skipAll()
	opts.Paths = testPaths(root, home)
	opts.SettingsPath = writeSettings(t, home, "task@nerv", "task-teamwork@nerv", "task-other@nerv")

	var out bytes.Buffer
	in := promptInput(&out, []promptAnswer{
		{prompt: "Task provider (other|teamwork) [teamwork]: ", answer: "other"},
		{prompt: "Write to ", answer: "y"},
	})
	if _, err := wizard.Run(deps, in, &out, opts); err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	if got, _ := scalarOf(t, opts.Paths.Config, "tasks.provider"); got != "other" {
		t.Errorf("tasks.provider = %q, want other", got)
	}
	if !strings.Contains(out.String(), "-- other --") || !strings.Contains(out.String(), "-- teamwork --") {
		t.Errorf("both providers' keys not prompted:\n%s", out.String())
	}
}

// S3: the repo flow prompts the enabled providers' repo-scope scalar keys and
// writes the answered ones into the repo's .ai-workflow.yml.
func TestRun_RepoInit_PromptsRepoScopeProviderKeys(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFixture(t, filepath.Join(root, "config.yml"))
	repoDir := t.TempDir()
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"git -C " + repoDir + " rev-parse --show-toplevel": {Stdout: repoDir + "\n"},
	}}
	deps := configure.Deps{Home: home, FS: nerv.PluginFS(), ExtensionsFS: nerv.ExtensionsFS(), Runner: runner, Now: fixedNow, LookPath: lookPathNone}
	opts := wizard.Options{Paths: testPaths(root, home), SkipSkills: true, SkipModels: true, NoRefresh: true}
	opts.SettingsPath = writeSettings(t, home, "task@nerv", "task-teamwork@nerv")

	var out bytes.Buffer
	in := promptInput(&out, []promptAnswer{
		{prompt: "Initialize a repository for NERV now?", answer: repoDir},
		{prompt: "Base branch for this repo", answer: ""},
		{prompt: "Teamwork project id of this repository (overrides default_project_id) []: ", answer: "111"},
		{prompt: "Teamwork tasklist id of this repository (overrides default_tasklist_id) []: ", answer: ""},
		{prompt: "Initialize a repository for NERV now?", answer: ""},
	})
	if _, err := wizard.Run(deps, in, &out, opts); err != nil {
		t.Fatalf("Run() error = %v; output:\n%s", err, out.String())
	}
	repoConfig := filepath.Join(repoDir, ".ai-workflow.yml")
	if got, _ := scalarOf(t, repoConfig, "tasks.providers.teamwork.project_id"); got != "111" {
		t.Errorf("project_id = %q, want 111", got)
	}
	if _, found := scalarOf(t, repoConfig, "tasks.providers.teamwork.tasklist_id"); found {
		t.Error("a blank answer was written")
	}
	if got, _ := scalarOf(t, repoConfig, "enabled"); got != "true" {
		t.Errorf("enabled = %q, want true", got)
	}
}
