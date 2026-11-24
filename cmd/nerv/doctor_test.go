package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/skills"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
	"github.com/war-apps/nerv-kit-by-gentle-ai/runner/runnertest"
)

// doctorWorld is a home where NERV is installed and set up, and a
// repository at top using it, with every tool found.
type doctorWorld struct {
	t         *testing.T
	home, top string
	opts      options
	runner    *runnertest.FakeRunner
}

func newDoctorWorld(t *testing.T) *doctorWorld {
	t.Helper()
	w := &doctorWorld{t: t, home: t.TempDir(), top: filepath.Join(t.TempDir(), "repo")}
	w.opts = testOptions(w.home)
	// Every tool is found except the NERV-ecosystem tools (nerv-task,
	// nerv-release).
	w.opts.LookPath = func(name string) (string, error) {
		if strings.HasPrefix(name, "nerv-") {
			return "", os.ErrNotExist
		}
		return "/usr/bin/" + name, nil
	}
	w.runner = &runnertest.FakeRunner{
		Default: runnertest.Response{ExitCode: 1},
		Responses: map[string]runnertest.Response{
			"gentle-ai --version":                                                {Stdout: "gentle-ai 4.2.0\n"},
			"git -C . rev-parse --show-toplevel":                                 {Stdout: w.top + "\n"},
			"git -C " + w.top + " remote get-url origin":                         {Stdout: "git@github.com:acme/widgets.git\n"},
			"git -C " + w.top + " rev-parse --verify --quiet refs/heads/develop": {Stdout: "0123abcd\n"},
		},
	}
	w.opts.Runner = w.runner

	want, err := version.PluginVersion(w.opts.PluginFS)
	if err != nil {
		t.Fatal(err)
	}
	p := paths.Resolve(w.home)
	w.write(p.InstalledPlugins, `{"plugins":{"nerv@nerv":[{"installPath":"/cache/nerv","version":"`+want+`"}]}}`)
	w.write(p.Settings, `{"enabledPlugins":{"nerv@nerv":true}}`)
	manifest, err := skills.LoadManifestFS(w.opts.PluginFS)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range manifest.Skills {
		w.write(filepath.Join(p.SkillsDir, s.Name, "SKILL.md"), "# "+s.Name+"\n")
	}
	w.write(filepath.Join(w.top, ".ai-workflow", "config.yaml"), "enabled: true\n")
	return w
}

func (w *doctorWorld) write(path, text string) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *doctorWorld) doctor(args ...string) (int, string, string) {
	w.t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{"doctor"}, args...), &out, &errOut, w.opts)
	return code, out.String(), errOut.String()
}

// Everything in order: every check passes, the optional parts are listed
// as such, there is no step to take, and the exit code is 0.
func TestDoctor_Healthy(t *testing.T) {
	w := newDoctorWorld(t)
	code, out, errOut := w.doctor()
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
	}
	contains(t, "the report", out,
		"This machine:\n  [ok]  gentle-ai    ",
		"  [ok]  plugin       nerv@nerv ",
		"\nThis repository ("+w.top+"):\n",
		"  [ok]  config       .ai-workflow/config.yaml, enabled\n",
		"  [ok]  migration    nothing to migrate\n",
		"  [ok]  origin       git@github.com:acme/widgets.git\n",
		"  [ok]  base branch  develop\n",
		": healthy\n")
	lacks(t, "the report", out, "Next steps:", "Optional:", "[!!]", "[xx]")
}

// Problems on both sides: the failures exit 1 and their steps come first,
// numbered, then the warnings'; nothing is written.
func TestDoctor_ListsTheStepsFailuresFirst(t *testing.T) {
	w := newDoctorWorld(t)
	delete(w.runner.Responses, "gentle-ai --version")
	delete(w.runner.Responses, "git -C "+w.top+" rev-parse --verify --quiet refs/heads/develop")
	before := treeHash(t, w.top)

	code, out, _ := w.doctor()
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, out)
	}
	contains(t, "the report", out,
		"  [xx]  gentle-ai    not found on PATH\n",
		"  [!!]  base branch  develop does not exist\n",
		": not ready\n",
		"Next steps:\n  1. Install gentle-ai 4.x (https://github.com/Gentleman-Programming/gentle-ai)\n  2. Create the develop branch, or set git.base_branch in .ai-workflow/config.yaml\n")
	if treeHash(t, w.top) != before {
		t.Error("nerv doctor changed the repository")
	}
}

// The user file's tasks:, release: and deploy: keys no longer apply at run
// time, except the personal ones: a user tasks.provider is not the
// repository's provider, and the user config check warns with the step
// that moves it under defaults:.
func TestDoctor_UserRunTimeKeysNoLongerApply(t *testing.T) {
	w := newDoctorWorld(t)
	user := paths.Resolve(w.home).UserConfig
	w.write(user, "tasks:\n  provider: teamwork\n  providers:\n    teamwork:\n      assignee_id: 7\n      project_id: 1\n")
	before := treeHash(t, w.home)

	code, out, _ := w.doctor()
	if code != 0 {
		t.Fatalf("exit %d, want 0 (a warning)\n%s", code, out)
	}
	contains(t, "the report", out,
		"  [!!]  user config  tasks.provider, tasks.providers.teamwork.project_id no longer apply: tasks, release and deploy are read from each project's .ai-workflow/config.yaml\n",
		// nerv migrate copies the user's tasks.provider into the project.
		"  [!!]  migration    nerv migrate has 2 step(s) to take\n",
		"Next steps:\n  1. Run nerv migrate --user --dry-run to see the steps, then nerv migrate --user\n  2. Run nerv migrate --dry-run to see the steps, then nerv migrate\n")
	if treeHash(t, w.home) != before {
		t.Error("nerv doctor changed the home")
	}
}

// Personal keys (scope: user provider keys, the timer store) and defaults:
// are where they belong: no warning, and a default never applies.
func TestDoctor_UserPersonalKeysAndDefaultsAreFine(t *testing.T) {
	w := newDoctorWorld(t)
	user := paths.Resolve(w.home).UserConfig
	w.write(user, "tasks:\n  timer_store: ~/t.json\n  providers:\n    teamwork:\n      assignee_id: 7\ndeploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\ndefaults:\n  tasks:\n    provider: teamwork\n")

	code, out, _ := w.doctor()
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	contains(t, "the report", out,
		"  [ok]  user config  "+user+"\n")
	lacks(t, "the report", out, "nerv migrate --user")
}

// nerv doctor no longer checks the release:, deploy: and tasks: sections,
// the extension plugins they needed or the project's own extensions and
// providers: the NERV-ecosystem tools own them now (S5).
func TestDoctor_NoExtensionChecks(t *testing.T) {
	w := newDoctorWorld(t)
	w.write(paths.Resolve(w.home).Settings, `{"enabledPlugins":{"nerv@nerv":true,"task@nerv":true}}`)
	w.write(filepath.Join(w.top, ".ai-workflow", "config.yaml"), "enabled: true\nrelease:\n  projects:\n    - name: api\n      path: .\n      build: bash\ndeploy:\n  targets: []\ntasks:\n  provider: teamwork\n")
	w.write(filepath.Join(w.top, ".ai-workflow", "providers", "task-acme", "provider.yaml"), "name: acme\n")
	code, out, _ := w.doctor()
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	contains(t, "the report", out, "  [ok]  base branch  develop\n", ": healthy\n")
	lacks(t, "the report", out, "]  release ", "]  deploy ", "]  tasks ", "project extensions", "nerv install --with", "/release:")
}

// The extension plugins nerv no longer ships, enabled in the repository's
// own Claude Code settings (nerv install and upgrade clean only the user
// settings), are reported with the files and the step that removes them.
func TestDoctor_ProjectSettingsOrphansAreReported(t *testing.T) {
	w := newDoctorWorld(t)
	w.write(filepath.Join(w.top, ".claude", "settings.json"), `{"enabledPlugins":{"task@nerv":true,"other@elsewhere":true}}`)
	w.write(filepath.Join(w.top, ".claude", "settings.local.json"), `{"enabledPlugins":{"release@nerv":false,"release-github@nerv":true}}`)
	code, out, _ := w.doctor()
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	contains(t, "the report", out,
		"  [!!]  old plugins  .claude/settings.json: task@nerv; .claude/settings.local.json: release@nerv, release-github@nerv (nerv no longer ships them)\n",
		"Remove them from enabledPlugins in those files; nerv-task and nerv-release replace them (nerv install --tools nerv-task,nerv-release)")
	lacks(t, "the report", out, "other@elsewhere")
}

// Without old plugins in the repository's settings there is no such row.
func TestDoctor_NoProjectSettingsOrphansNoRow(t *testing.T) {
	w := newDoctorWorld(t)
	w.write(filepath.Join(w.top, ".claude", "settings.json"), `{"enabledPlugins":{"nerv@nerv":true}}`)
	_, out, _ := w.doctor()
	lacks(t, "the report", out, "old plugins")
}

// A 4.2.0 repository: nerv migrate is the step, listed once although two
// checks name it, and the checks that read the configuration are skipped.
func TestDoctor_OldConfigurationPointsToMigrate(t *testing.T) {
	w := newDoctorWorld(t)
	if err := os.RemoveAll(filepath.Join(w.top, ".ai-workflow")); err != nil {
		t.Fatal(err)
	}
	w.write(filepath.Join(w.top, ".nerv", "nerv.yaml"), "enabled: true\n")
	code, out, _ := w.doctor()
	if code != 0 {
		t.Fatalf("exit %d, want 0 (warnings only)\n%s", code, out)
	}
	contains(t, "the report", out,
		"  [!!]  config       .nerv/nerv.yaml is the nerv 4.2.0 configuration\n",
		"  [!!]  migration    nerv migrate has ",
		": needs attention\n",
		"Next steps:\n  1. "+stepMigrate+"\n")
	lacks(t, "the report", out, "  2. ", "release ", "base branch")
}

// Outside a repository the repository checks are skipped, not failed.
func TestDoctor_OutsideARepository(t *testing.T) {
	w := newDoctorWorld(t)
	w.runner.Responses["git -C . rev-parse --show-toplevel"] = runnertest.Response{ExitCode: 128, Stderr: "fatal: not a git repository"}
	code, out, _ := w.doctor()
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	contains(t, "the report", out, "This repository:\n  [--]  repository   . is not inside a git repository (fatal: not a git repository): the repository checks were skipped\n")
}

// --json prints the same results with their section, status and step.
func TestDoctor_JSON(t *testing.T) {
	w := newDoctorWorld(t)
	delete(w.runner.Responses, "git -C "+w.top+" rev-parse --verify --quiet refs/heads/develop")
	code, out, _ := w.doctor("--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	var got struct {
		Health  string `json:"health"`
		Results []struct {
			Section, Name, Status, Detail, Step string
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got.Health != "needs attention" {
		t.Errorf("health = %q", got.Health)
	}
	last := got.Results[len(got.Results)-1]
	if last.Section != "This repository ("+w.top+")" || last.Name != "base branch" || last.Status != "warn" ||
		last.Detail != "develop does not exist" || !strings.Contains(last.Step, "set git.base_branch") {
		t.Errorf("last result = %+v", last)
	}
}

func TestDoctor_Usage(t *testing.T) {
	w := newDoctorWorld(t)
	if code, out, _ := w.doctor("--help"); code != 0 || !strings.HasPrefix(out, "Usage: nerv doctor") {
		t.Errorf("--help: exit %d\n%s", code, out)
	}
	if code, _, errOut := w.doctor("now"); code != 2 || !strings.HasPrefix(errOut, `nerv doctor: unexpected argument "now"`) {
		t.Errorf("an argument: exit %d, stderr %q", code, errOut)
	}
	if code, _, errOut := w.doctor("--fix"); code != 2 || !strings.HasPrefix(errOut, "nerv doctor: flag provided but not defined: -fix") {
		t.Errorf("an unknown flag: exit %d, stderr %q", code, errOut)
	}
}

func contains(t *testing.T, what, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("%s does not contain %q:\n%s", what, want, text)
		}
	}
}

func lacks(t *testing.T, what, text string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(text, u) {
			t.Errorf("%s still contains %q:\n%s", what, u, text)
		}
	}
}

// treeHash hashes every file under dir (path and content).
func treeHash(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(path)
			h.Write([]byte(path + "\x00"))
			h.Write(data)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}
