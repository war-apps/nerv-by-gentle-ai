package releaseconfig_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
)

// loadProblems loads text and returns the validation problems, failing the
// test when Load does not refuse it.
func loadProblems(t *testing.T, text string) string {
	t.Helper()
	_, err := releaseconfig.Load(writeConfig(t, text))
	var invalid *releaseconfig.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("Load = %v, want a *ValidationError", err)
	}
	return strings.Join(invalid.Problems, "\n")
}

// S4, S7: the release: section selects the providers of its slots and
// holds their keys as written (each provider checks its own); its projects
// keep only what is the release's, and an unset tag_format or changelog
// is left to the version engine.
func TestLoad_ReleaseSectionSelectsItsProviders(t *testing.T) {
	rel := mustLoad(t, `release:
  version: conventional-commits
  build: bash
  providers:
    conventional-commits:
      branches:
        - main
        - { name: next, prerelease: rc, channel: beta }
      tag_format: "release-${version}"
    bash: { shared_host: true }
  projects:
    - name: core
      path: src/core
    - name: api
      path: src/api
      shared_paths: [Directory.Build.props]
      scopes: [api, web]
      depends_on:
        - { project: core, bump: major }
`)
	if rel.Version != "conventional-commits" || rel.Build != "bash" {
		t.Errorf("version %q, build %q", rel.Version, rel.Build)
	}
	engine := rel.Providers["conventional-commits"]
	if engine["tag_format"] != "release-${version}" || len(engine["branches"].([]any)) != 2 {
		t.Errorf("providers.conventional-commits = %v", engine)
	}
	if len(rel.Projects) != 2 {
		t.Fatalf("projects = %+v", rel.Projects)
	}
	api := rel.Projects[1]
	if api.Path != "src/api" || strings.Join(api.SharedPaths, ",") != "Directory.Build.props" || strings.Join(api.Scopes, ",") != "api,web" ||
		len(api.DependsOn) != 1 || api.DependsOn[0] != (releaseconfig.Dependency{Project: "core", Bump: "major"}) {
		t.Errorf("api = %+v", api)
	}
	if api.TagFormat != "" || api.Changelog != "" {
		t.Errorf("api tag %q changelog %q, want them left to the engine", api.TagFormat, api.Changelog)
	}
	if core := rel.Projects[0]; strings.Join(core.Scopes, ",") != "core" {
		t.Errorf("core scopes = %v, want the project's name", core.Scopes)
	}
}

// S7: the slot selections and provider names are names.
func TestLoad_ReleaseSectionRefusesInvalidProviderNames(t *testing.T) {
	got := loadProblems(t, `release:
  version: Conventional Commits
  build: "a b"
  providers:
    Bash: {}
  projects:
    - name: api
      path: .
`)
	for _, want := range []string{
		`release.version: "Conventional Commits" is not a provider name`,
		`release.build: "a b" is not a provider name`,
		"release.providers.Bash: not a provider name",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("problems lack %q:\n%s", want, got)
		}
	}
}

// S1: a release: section with a 4.2.0 key (semantic_release:, a preset)
// is the 4.2.0 section: the refusal names the keys and nerv migrate.
func TestLoad_LegacyReleaseSectionIsRefused(t *testing.T) {
	for _, tt := range []struct{ name, yaml, key string }{
		{"semantic_release", "release:\n  semantic_release: { version: 24.2.3 }\n  projects:\n    - name: api\n      path: .\n      build: bash\n", "release.semantic_release"},
		{"presets", "release:\n  projects:\n    - name: api\n      path: .\n      presets: [dotnet]\n", "release.projects[0].presets"},
		{"preset settings", "release:\n  projects:\n    - name: web\n      path: web\n      build: bash\n    - name: api\n      path: .\n      docker: { images: [] }\n", "release.projects[1].docker"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := loadProblems(t, tt.yaml)
			for _, want := range []string{"release: this is a 4.2.0 release: section", tt.key, "run nerv migrate"} {
				if !strings.Contains(got, want) {
					t.Errorf("refusal lacks %q:\n%s", want, got)
				}
			}
		})
	}
}

// S1: LegacyKeys tells the 4.2.0 section apart from today's, which shares
// its key.
func TestLegacyKeys(t *testing.T) {
	for _, tt := range []struct {
		name, yaml string
		want       []string
	}{
		{"today's section", "release:\n  providers:\n    conventional-commits: { branches: [{ name: main }] }\n  projects:\n    - name: api\n      path: .\n      build: bash\n", nil},
		{"no section", "enabled: true\n", nil},
		{"4.2.0 section", "release:\n  semantic_release: { version: 24.2.3 }\n  projects:\n    - name: api\n      path: .\n      presets: [dotnet, nuget]\n      dotnet: { project_file: Api.csproj }\n",
			[]string{"release.semantic_release", "release.projects[0].presets", "release.projects[0].dotnet"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := releaseconfig.LegacyKeys([]byte(tt.yaml)); !slices.Equal(got, tt.want) {
				t.Errorf("LegacyKeys = %q, want %q", got, tt.want)
			}
		})
	}
}

// S9: release.semantic_release has no equivalent.
func TestLoad_SemanticReleaseKeyIsRefused(t *testing.T) {
	got := loadProblems(t, "release:\n  semantic_release: { version: 24.2.3 }\n  projects:\n    - name: api\n      path: .\n      build: bash\n")
	if !strings.Contains(got, "release.semantic_release: removed") || !strings.Contains(got, "natively") {
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
release:
  projects:
    - name: api
      path: .
      build: bash
`)
	for _, want := range []string{
		"pipeline: section is no longer read",
		"validate and test: nerv release",
		"docker and release: nerv release, then nerv deploy",
		"deploy: the release-ssh-compose target",
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
	if got := releaseconfig.RemovedSections([]byte("release:\n  projects: []\ndeploy:\n  targets: []\n")); len(got) != 0 {
		t.Errorf("RemovedSections = %q, want none", got)
	}
	got := strings.Join(releaseconfig.RemovedSections([]byte("pipeline:\n  test: { run: x }\n")), "\n")
	if !strings.Contains(got, "validate and test: nerv release") || strings.Contains(got, "sync") {
		t.Errorf("RemovedSections = %q", got)
	}
}

// L10: the publish hooks point to nerv deploy.
func TestLoad_PublishHooksPointToDeploy(t *testing.T) {
	got := loadProblems(t, `release:
  projects:
    - name: api
      path: .
      build: bash
      hooks:
        before_publish: [{ run: echo }]
        after_publish: [{ run: echo }]
`)
	for _, key := range []string{"before_publish", "after_publish"} {
		if !strings.Contains(got, "release.projects[0].hooks."+key+": ") || !strings.Contains(got, "nerv deploy") {
			t.Errorf("problems for %s:\n%s", key, got)
		}
	}
}

// A section without projects configures the engine for the whole
// repository (nerv version next); nerv release needs projects.
func TestLoad_SectionWithoutProjects(t *testing.T) {
	for _, text := range []string{"enabled: true\nrelease:\n", "release:\n  providers:\n    conventional-commits: { tag_format: \"r${version}\" }\n"} {
		rel := mustLoad(t, text)
		if len(rel.Projects) != 0 {
			t.Errorf("%q: projects %+v", text, rel.Projects)
		}
		err := rel.CheckBuild()
		var invalid *releaseconfig.ValidationError
		if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "release.projects: nerv release needs at least one project") {
			t.Errorf("%q: CheckBuild = %v", text, err)
		}
	}
}

// B2 (L21): the pipeline: refusal names every real 4.2.0 key, with where it
// went, and points to nerv migrate.
func TestRemovedSections_PipelineNamesEveryRealKey(t *testing.T) {
	data := []byte("pipeline:\n  dotnet: { solution: App.slnx }\n  branches:\n    - { name: main }\n  registry: { host: ghcr.io }\n  images:\n    - { name: ghcr.io/acme/app }\n  release: { version_files: [App.csproj] }\n  deploy: { source: env, environments: [staging] }\n  sync: { enabled: true }\n")
	msg := strings.Join(releaseconfig.RemovedSections(data), "\n")
	for _, key := range []string{"dotnet", "branches", "registry", "images", "release", "deploy", "sync"} {
		if !strings.Contains(msg, "pipeline."+key) {
			t.Errorf("message does not say where pipeline.%s went:\n%s", key, msg)
		}
	}
	for _, want := range []string{"build: bash", "release.providers.conventional-commits.branches", "project's images", "release-ssh-compose", "project's version_files", "not ported", "run nerv migrate"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
}
