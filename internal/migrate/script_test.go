package migrate_test

import (
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/deployconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
)

// The deploy script target is script in the release extension: nerv migrate
// rewrites provider: bash in the project's deploy targets, block and flow
// style, comments kept, and leaves the release build provider alone.
func TestScriptTarget_ProjectTargetsAreRenamed(t *testing.T) {
	project := "enabled: true\nrelease:\n  build: bash\ndeploy:\n  host: github\n  targets:\n    - name: lab\n      provider: bash # our script\n      config: { script: deploy/lab.sh }\n    - { name: prod, provider: \"bash\", config: { script: deploy/prod.sh } }\n    - name: box\n      provider: ssh-compose\n      config: { environment: box }\n"
	w := userWorld(t, "", project)
	p, _ := w.migrate(w.options(false, ""))
	want := "enabled: true\nrelease:\n  build: bash\ndeploy:\n  host: github\n  targets:\n    - name: lab\n      provider: script # our script\n      config: { script: deploy/lab.sh }\n    - { name: prod, provider: \"script\", config: { script: deploy/prod.sh } }\n    - name: box\n      provider: ssh-compose\n      config: { environment: box }\n"
	if got := w.read(".ai-workflow/config.yaml"); got != want {
		t.Errorf(".ai-workflow/config.yaml =\n%s\nwant\n%s", got, want)
	}
	if !anyContains(p.Notes, ".ai-workflow/config.yaml: deploy target provider bash renamed script") {
		t.Errorf("the rename is not reported: %v", p.Notes)
	}
	dep, err := deployconfig.Load(w.root)
	if err != nil {
		t.Fatalf("the migrated deploy: section does not load: %v", err)
	}
	if dep.Targets[0].Provider != "script" || dep.Targets[1].Provider != "script" || dep.Targets[2].Provider != "ssh-compose" {
		t.Errorf("targets %+v", dep.Targets)
	}
	if p2 := w.plan(w.options(false, "")); !p2.Empty() {
		t.Errorf("a second run changes %v", p2.Changes)
	}
}

// The user's defaults.deploy seed is renamed too.
func TestScriptTarget_UserDefaultsAreRenamed(t *testing.T) {
	w := userWorld(t, "git:\n  worktree: ask\ndefaults:\n  release:\n    build: bash\n  deploy:\n    targets:\n      - name: lab\n        provider: bash\n", "")
	p, _ := w.migrate(userOnly(w))
	want := "git:\n  worktree: ask\ndefaults:\n  release:\n    build: bash\n  deploy:\n    targets:\n      - name: lab\n        provider: script\n"
	if got := w.read(w.userPaths().UserConfig); got != want {
		t.Errorf("user config =\n%s\nwant\n%s", got, want)
	}
	if !anyContains(p.Notes, "user config: defaults.deploy: deploy target provider bash renamed script") {
		t.Errorf("the rename is not reported: %v", p.Notes)
	}
}

// A user deploy: block that still holds targets moves under
// defaults.deploy renamed, in one run: a second run changes nothing.
func TestScriptTarget_UserDeployBlockIsRenamedBeforeItMoves(t *testing.T) {
	w := userWorld(t, "deploy:\n  targets:\n    - { name: s, provider: bash }\n", "")
	w.migrate(userOnly(w))
	got := w.read(w.userPaths().UserConfig)
	if !strings.Contains(got, "provider: script") || strings.Contains(got, "provider: bash") || !strings.Contains(got, "defaults:") {
		t.Errorf("user config =\n%s", got)
	}
	if p2 := w.plan(userOnly(w)); !p2.Empty() {
		t.Errorf("a second run changes %v", p2.Changes)
	}
}

// A project deploy provider, .ai-workflow/providers/deploy-<name>, moves to
// release-<name> with every file, and its extension: deploy becomes
// release; a dry run writes nothing.
func TestProjectDeployProviders_MoveToTheReleaseExtension(t *testing.T) {
	manifest := "name: rsync\nextension: deploy # moved\nslot: target\ncontract_version: 1\nruntime: exec\noperations:\n  deploy:\n    - argv: [bash, \"{provider_dir}/deploy.sh\"]\n"
	w := userWorld(t, "", "enabled: true\ndeploy:\n  targets:\n    - name: lab\n      provider: rsync\n")
	w.write(".ai-workflow/providers/deploy-rsync/provider.yaml", manifest)
	w.write(".ai-workflow/providers/deploy-rsync/deploy.sh", "#!/bin/bash\necho deployed\n")
	w.write(".ai-workflow/providers/task-board/provider.yaml", "name: board\n")
	before := treeHash(t, w.root)
	p := w.plan(w.options(false, ""))
	if treeHash(t, w.root) != before {
		t.Fatal("planning changed the files")
	}
	w.migrate(w.options(false, ""))
	if got := w.read(".ai-workflow/providers/release-rsync/provider.yaml"); got != strings.Replace(manifest, "extension: deploy", "extension: release", 1) {
		t.Errorf("release-rsync/provider.yaml =\n%s", got)
	}
	if got := w.read(".ai-workflow/providers/release-rsync/deploy.sh"); got != "#!/bin/bash\necho deployed\n" {
		t.Errorf("release-rsync/deploy.sh = %q", got)
	}
	if w.exists(".ai-workflow/providers/deploy-rsync") {
		t.Error(".ai-workflow/providers/deploy-rsync is still there")
	}
	if !w.exists(".ai-workflow/providers/task-board/provider.yaml") {
		t.Error("a task provider moved")
	}
	if !anyContains(p.Notes, ".ai-workflow/providers/deploy-rsync moved to release-rsync") {
		t.Errorf("the move is not reported: %v", p.Notes)
	}
}

// A destination that exists, or a provider named bash, is refused before
// anything is written.
func TestProjectDeployProviders_ConflictsAreRefused(t *testing.T) {
	for name, files := range map[string][]string{
		"release-rsync already exists": {".ai-workflow/providers/deploy-rsync/provider.yaml", ".ai-workflow/providers/release-rsync/provider.yaml"},
		"would be release-bash":        {".ai-workflow/providers/deploy-bash/provider.yaml"},
	} {
		w := userWorld(t, "", "enabled: true\n")
		for _, f := range files {
			w.write(f, "name: x\nextension: deploy\n")
		}
		before := treeHash(t, w.root)
		_, err := migrate.Compute(w.options(false, ""))
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: Compute = %v", name, err)
		}
		if treeHash(t, w.root) != before {
			t.Errorf("%s: a refused migration changed the files", name)
		}
	}
}
