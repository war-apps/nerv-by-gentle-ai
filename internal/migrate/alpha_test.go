package migrate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
)

const alpha1 = "alpha1"

// alpha1Release is the release: section the alpha1 fixture's ship: section
// becomes.
const alpha1Release = `release: # ship plugin; project scope only
  providers:
    conventional-commits:
      # The version engine.
      branches:
        - main
        - {name: develop, prerelease: alpha, channel: alpha}
      release_rules:
        - {type: docs, scope: readme, release: patch}
      note_keywords: [BREAKING CHANGE, BREAKING]
      tag_format: "v${version}"
      repository_url: https://github.com/acme/widgets
      changelog: {file: CHANGELOG.md, title: "# Changelog"}
  projects:
    - name: api
      path: api
      build: docker-compose
      images: ["ghcr.io/acme/api:${version}"]
      providers:
        docker-compose:
          file: api/docker-compose.yml
          test_service: tests
    - name: worker
      path: worker
      tag_format: "worker-v${version}"
      changelog: worker/HISTORY.md
      depends_on:
        - {project: api, bump: patch}
      build: bash
      version_files: [worker/Worker.csproj]
      coverage: {line: 80}
      providers:
        bash:
          shared_host: true
      stages:
        restore: {run: dotnet restore worker}
        build: {run: dotnet build worker -c Release --no-restore}
        test: {run: dotnet test worker --no-build}
        package: {run: 'dotnet publish worker -o "$NERV_RELEASE_OUTPUTS_DIR"'}
      hooks:
        prepare:
          - {run: echo ready}
`

// S1, S7, S9: an alpha.1 .ai-workflow.yml with a ship: section ends as
// .ai-workflow/config.yaml with today's release: section, in one
// transaction: the dry run writes nothing, the old file is backed up, the
// result loads with each key in its new place, and a second run is a
// no-op.
func TestAlpha_ShipSectionBecomesRelease(t *testing.T) {
	w := newWorld(t, alpha1)
	old := w.read(".ai-workflow.yml")
	before := treeHash(t, w.root)

	p := w.plan(w.options(false, githubOrigin))
	contains(t, "the rendered plan", migrate.Render(p), "Create .ai-workflow/config.yaml: the new project configuration, from .ai-workflow.yml\n", "Delete .ai-workflow.yml: replaced by .ai-workflow/config.yaml")
	contains(t, "the notes", strings.Join(p.Notes, "\n"),
		"ship: renamed release: (the 5.0.0-alpha.1 section)",
		"release.branches: moved to release.providers.conventional-commits.branches",
		"release.projects[0].build.mode: compose became release.projects[0].build: docker-compose",
		"release.projects[0].build.file: moved to release.projects[0].providers.docker-compose.file",
		"release.projects[1].build.shared_host: moved to release.projects[1].providers.bash.shared_host",
		"release.projects[1].build.coverage: moved to release.projects[1].coverage")
	if treeHash(t, w.root) != before {
		t.Fatal("planning changed the files")
	}

	w.migrate(w.options(false, githubOrigin))
	text := w.read(".ai-workflow/config.yaml")
	head := "# .ai-workflow/config.yaml -- NERV project configuration (5.0.0-alpha.1)\nenabled: true\ngit:\n  base_branch: develop\n"
	tail := "deploy:                             # deploy plugin\n  host: github\n  targets:\n    - { name: staging, provider: ssh-compose, config: { environment: staging } }\n"
	if want := head + alpha1Release + tail; text != want {
		t.Errorf(".ai-workflow/config.yaml =\n%s\nwant\n%s", text, want)
	}
	if w.exists(".ai-workflow.yml") {
		t.Error(".ai-workflow.yml is still there")
	}
	backup := filepath.Join(w.root, ".nerv", "backups", "migrate-"+now.Format("20060102-150405"), ".ai-workflow.yml")
	if data, err := os.ReadFile(backup); err != nil || string(data) != old {
		t.Errorf("backup %s = %q, %v; want the old file", backup, data, err)
	}

	rel, err := releaseconfig.Load(w.root)
	if err != nil {
		t.Fatalf("the migrated section does not load: %v", err)
	}
	api, worker := rel.Projects[0], rel.Projects[1]
	if rel.Version != "" || rel.Providers["conventional-commits"]["tag_format"] != "v${version}" ||
		rel.Providers["conventional-commits"]["repository_url"] != "https://github.com/acme/widgets" {
		t.Errorf("engine: version %q, providers %v", rel.Version, rel.Providers)
	}
	if api.Build != "docker-compose" || api.Providers["docker-compose"]["file"] != "api/docker-compose.yml" ||
		api.Providers["docker-compose"]["test_service"] != "tests" || len(api.Images) != 1 || api.Images[0].Ref != "ghcr.io/acme/api:${version}" {
		t.Errorf("api = %+v", api)
	}
	if worker.Build != "bash" || worker.Providers["bash"]["shared_host"] != true || worker.TagFormat != "worker-v${version}" ||
		worker.Changelog != "worker/HISTORY.md" || len(worker.VersionFiles) != 1 || worker.Coverage == nil || worker.Coverage.Line != 80 {
		t.Errorf("worker = %+v", worker)
	}

	after := treeHash(t, w.root)
	p = w.plan(w.options(false, githubOrigin))
	if !p.Empty() {
		t.Errorf("the second plan has changes:\n%s", migrate.Render(p))
	}
	if _, err := migrate.Apply(p, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if treeHash(t, w.root) != after {
		t.Error("the second run changed files")
	}
}

// An alpha.1 ship: section already moved into .ai-workflow/config.yaml is
// converted there.
func TestAlpha_ShipSectionInTheFolderIsConverted(t *testing.T) {
	w := world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
	w.write(".ai-workflow/config.yaml", "enabled: true\nship:\n  tag_format: \"r${version}\"\n  projects:\n    - { name: api, path: ., build: { mode: script } }\n")
	w.migrate(w.options(false, ""))
	want := "enabled: true\nrelease:\n  providers:\n    conventional-commits:\n      tag_format: \"r${version}\"\n  projects:\n    - name: api\n      path: .\n      build: bash\n"
	if got := w.read(".ai-workflow/config.yaml"); got != want {
		t.Errorf(".ai-workflow/config.yaml =\n%s\nwant\n%s", got, want)
	}
}

// A file with both ship: and release:, and a ship: section with no place
// for one of its keys, are refused, and nothing is written.
func TestAlpha_RefusedShipSections(t *testing.T) {
	for _, tt := range []struct{ name, text, want string }{
		{"both sections", "ship:\n  projects: []\nrelease:\n  projects: []\n", "both a ship: section (5.0.0-alpha.1) and a release: section"},
		{"unknown build mode", "ship:\n  projects:\n    - { name: api, path: ., build: { mode: maven } }\n", `release.projects[0].build.mode: unknown build mode "maven" (want compose or script)`},
		{"unknown build key", "ship:\n  projects:\n    - { name: api, path: ., build: { mode: script, sdk: \"8\" } }\n", "release.projects[0].build.sdk: unknown build key"},
		{"engine key set twice", "ship:\n  tag_format: \"a${version}\"\n  providers:\n    conventional-commits: { tag_format: \"b${version}\" }\n", "release.providers.conventional-commits.tag_format is set too"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
			w.write(".ai-workflow/config.yaml", tt.text)
			before := treeHash(t, w.root)
			_, err := migrate.Compute(w.options(false, ""))
			if !refusal.Is(err) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Compute = %v, want a refusal naming %q", err, tt.want)
			}
			if treeHash(t, w.root) != before {
				t.Error("a refused migration changed files")
			}
		})
	}
}

// A release: section with the version engine keys at its top and a
// build: mapping (written between the ship -> release rename and the
// version and build slots) is moved the same way, as Load's refusal of it
// says.
func TestAlpha_ReleaseSectionWithMovedKeysIsConverted(t *testing.T) {
	w := world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
	w.write(".ai-workflow/config.yaml", "release:\n  branches: [main]\n  projects:\n    - name: api\n      path: .\n      build: { mode: compose, file: c.yml }\n")
	w.migrate(w.options(false, ""))
	want := "release:\n  providers:\n    conventional-commits:\n      branches: [main]\n  projects:\n    - name: api\n      path: .\n      build: docker-compose\n      providers:\n        docker-compose:\n          file: c.yml\n"
	if got := w.read(".ai-workflow/config.yaml"); got != want {
		t.Errorf(".ai-workflow/config.yaml =\n%s\nwant\n%s", got, want)
	}
}
