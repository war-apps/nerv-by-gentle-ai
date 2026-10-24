package shipconfig_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

// loadProblems loads text and returns the validation problems, failing the
// test when Load does not refuse it.
func loadProblems(t *testing.T, text string) string {
	t.Helper()
	_, err := shipconfig.Load(writeConfig(t, text))
	var invalid *shipconfig.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("Load = %v, want a *ValidationError", err)
	}
	return strings.Join(invalid.Problems, "\n")
}

// S2, S3: the ship: section embeds the version engine's settings, and its
// projects are the engine's projects (one list).
func TestLoad_ShipSectionEmbedsTheVersionEngine(t *testing.T) {
	rel := mustLoad(t, `ship:
  branches:
    - main
    - { name: next, prerelease: rc, channel: beta }
  release_rules:
    - { type: docs, release: patch }
  note_keywords: [BREAKING]
  tag_format: "release-${version}"
  repository_url: https://example.com/org/repo
  changelog: { file: HISTORY.md, title: "# History" }
  projects:
    - name: core
      path: src/core
      build: { mode: script }
    - name: api
      path: src/api
      shared_paths: [Directory.Build.props]
      scopes: [api, web]
      depends_on:
        - { project: core, bump: major }
      build: { mode: script }
`)
	cfg := rel.Versioning()
	if len(cfg.Branches) != 2 || cfg.Branches[1].Name != "next" || cfg.Branches[1].Prerelease.ID() != "rc" || cfg.Branches[1].Channel != "beta" {
		t.Errorf("branches = %+v", cfg.Branches)
	}
	if len(cfg.ReleaseRules) != 1 || cfg.ReleaseRules[0].Type != "docs" || cfg.ReleaseRules[0].Release != "patch" {
		t.Errorf("release_rules = %+v", cfg.ReleaseRules)
	}
	if strings.Join(cfg.NoteKeywords, ",") != "BREAKING" || cfg.TagFormat != "release-${version}" ||
		cfg.RepositoryURL != "https://example.com/org/repo" || cfg.Changelog.Title != "# History" {
		t.Errorf("engine settings = %+v", cfg)
	}
	if len(cfg.Projects) != 2 {
		t.Fatalf("projects = %+v", cfg.Projects)
	}
	api := cfg.Projects[1]
	want := versioning.Project{
		Name: "api", Path: "src/api", SharedPaths: []string{"Directory.Build.props"}, Scopes: []string{"api", "web"},
		TagFormat: "api-release-${version}", Changelog: "src/api/HISTORY.md",
		DependsOn: []versioning.Dependency{{Project: "core", Bump: "major"}},
	}
	if api.Name != want.Name || api.Path != want.Path || strings.Join(api.SharedPaths, ",") != "Directory.Build.props" ||
		strings.Join(api.Scopes, ",") != "api,web" || api.TagFormat != want.TagFormat || api.Changelog != want.Changelog ||
		len(api.DependsOn) != 1 || api.DependsOn[0] != want.DependsOn[0] {
		t.Errorf("api = %+v, want %+v", api, want)
	}
	// The ship project carries the same defaults.
	p, _ := rel.Project("api")
	if p.TagFormat != "api-release-${version}" || p.Changelog != "src/api/HISTORY.md" {
		t.Errorf("ship project = tag %q changelog %q", p.TagFormat, p.Changelog)
	}
}

// S2: an invalid engine setting is refused with its ship.-qualified path.
func TestLoad_ShipSectionRefusesInvalidEngineSettings(t *testing.T) {
	got := loadProblems(t, `ship:
  release_rules:
    - { type: docs, release: huge }
  projects:
    - name: api
      path: .
      build: { mode: script }
`)
	if !strings.Contains(got, `ship.release_rules[0]: release "huge" is not major, minor, patch or none`) {
		t.Errorf("problems:\n%s", got)
	}
}

// S9: the old release: section is no longer read; the refusal says what to
// do.
func TestLoad_OldReleaseSectionIsRefused(t *testing.T) {
	got := loadProblems(t, "release:\n  projects:\n    - name: api\n      path: .\n")
	for _, want := range []string{"release:", "no longer read", "ship:", "nerv migrate", "nerv ship setup --import"} {
		if !strings.Contains(got, want) {
			t.Errorf("refusal lacks %q:\n%s", want, got)
		}
	}
}

// S9: release.semantic_release has no equivalent.
func TestLoad_SemanticReleaseKeyIsRefused(t *testing.T) {
	got := loadProblems(t, "ship:\n  semantic_release: { version: 24.2.3 }\n  projects:\n    - name: api\n      path: .\n      build: { mode: script }\n")
	if !strings.Contains(got, "ship.semantic_release: removed") || !strings.Contains(got, "natively") {
		t.Errorf("problems:\n%s", got)
	}
}

// S9: the old pipeline: section is refused with where each part went.
func TestLoad_OldPipelineSectionIsRefused(t *testing.T) {
	got := loadProblems(t, `pipeline:
  validate: { run: make lint }
  test: { run: make test }
  docker: { image: org/api }
  release: {}
  deploy:
    selection: branches
  sync: {}
ship:
  projects:
    - name: api
      path: .
      build: { mode: script }
`)
	for _, want := range []string{
		"pipeline: section is no longer read",
		"validate and test: nerv ship",
		"docker and release: nerv ship, then nerv deploy",
		"deploy: the deploy-ssh-compose target",
		"sync: not ported yet",
		"selection: branches",
		"nerv migrate",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("refusal lacks %q:\n%s", want, got)
		}
	}
}

// The pipeline: refusal is shared by every reader of the file.
func TestRemovedSections_ReportsOnlyWhatIsThere(t *testing.T) {
	if got := shipconfig.RemovedSections([]byte("ship:\n  projects: []\ndeploy:\n  targets: []\n")); len(got) != 0 {
		t.Errorf("RemovedSections = %q, want none", got)
	}
	got := strings.Join(shipconfig.RemovedSections([]byte("pipeline:\n  test: { run: x }\n")), "\n")
	if !strings.Contains(got, "validate and test: nerv ship") || strings.Contains(got, "sync") {
		t.Errorf("RemovedSections = %q", got)
	}
}

// L10: the publish hooks point to nerv deploy.
func TestLoad_PublishHooksPointToDeploy(t *testing.T) {
	got := loadProblems(t, `ship:
  projects:
    - name: api
      path: .
      build: { mode: script }
      hooks:
        before_publish: [{ run: echo }]
        after_publish: [{ run: echo }]
`)
	for _, key := range []string{"before_publish", "after_publish"} {
		if !strings.Contains(got, "ship.projects[0].hooks."+key+": ") || !strings.Contains(got, "nerv deploy") {
			t.Errorf("problems for %s:\n%s", key, got)
		}
	}
}

// A section without projects configures the engine for the whole
// repository (nerv version next); nerv ship needs projects.
func TestLoad_SectionWithoutProjects(t *testing.T) {
	for _, text := range []string{"enabled: true\nship:\n", "ship:\n  tag_format: r${version}\n"} {
		rel := mustLoad(t, text)
		if len(rel.Projects) != 0 || len(rel.Versioning().Projects) != 0 {
			t.Errorf("%q: projects %+v", text, rel.Projects)
		}
		err := rel.CheckBuild()
		var invalid *shipconfig.ValidationError
		if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "ship.projects: nerv ship needs at least one project") {
			t.Errorf("%q: CheckBuild = %v", text, err)
		}
	}
}

// B2 (L21): the pipeline: refusal names every real 4.2.0 key, with where it
// went, and points to nerv migrate.
func TestRemovedSections_PipelineNamesEveryRealKey(t *testing.T) {
	data := []byte("pipeline:\n  dotnet: { solution: App.slnx }\n  branches:\n    - { name: main }\n  registry: { host: ghcr.io }\n  images:\n    - { name: ghcr.io/acme/app }\n  release: { version_files: [App.csproj] }\n  deploy: { source: env, environments: [staging] }\n  sync: { enabled: true }\n")
	msg := strings.Join(shipconfig.RemovedSections(data), "\n")
	for _, key := range []string{"dotnet", "branches", "registry", "images", "release", "deploy", "sync"} {
		if !strings.Contains(msg, "pipeline."+key) {
			t.Errorf("message does not say where pipeline.%s went:\n%s", key, msg)
		}
	}
	for _, want := range []string{"script stages", "ship.branches", "build.images", "deploy-ssh-compose", "build.version_files", "not ported", "run nerv migrate"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
}
