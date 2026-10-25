package releaseconfig_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
)

// writeConfig writes text as <repo>/.ai-workflow.yml in a fresh temporary
// repository and returns the repository root.
func writeConfig(t *testing.T, text string) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".ai-workflow.yml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

// writeLegacyConfig writes text as the pre-rename <repo>/.nerv/nerv.yaml.
func writeLegacyConfig(t *testing.T, text string) string {
	t.Helper()
	repo := t.TempDir()
	dir := filepath.Join(repo, ".nerv")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nerv.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

func mustLoad(t *testing.T, text string) *releaseconfig.Release {
	t.Helper()
	rel, err := releaseconfig.Load(writeConfig(t, text))
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	return rel
}

func TestLoad_NotConfigured(t *testing.T) {
	tests := []struct {
		name string
		repo func(t *testing.T) string
	}{
		{"no .ai-workflow.yml", func(t *testing.T) string { return t.TempDir() }},
		// The old file is never read as configuration (nerv migrate moves it).
		{"only the old .nerv/nerv.yaml", func(t *testing.T) string {
			return writeLegacyConfig(t, "release:\n  projects:\n    - name: api\n      path: .\n      build: bash\n")
		}},
		{".ai-workflow.yml without release", func(t *testing.T) string {
			return writeConfig(t, "enabled: true\nmodels:\n  misato: { model: fable }\n")
		}},
		{"empty .ai-workflow.yml", func(t *testing.T) string { return writeConfig(t, "") }},
		{"comment-only .ai-workflow.yml", func(t *testing.T) string { return writeConfig(t, "# nothing yet\n") }},
		// .ai-workflow.yml is otherwise maintained by a line-based editor: a strict
		// YAML error outside release: must not matter to a repo without one.
		{"duplicate key elsewhere", func(t *testing.T) string {
			return writeConfig(t, "enabled: true\nenabled: false\n")
		}},
		{"tab indentation elsewhere", func(t *testing.T) string {
			return writeConfig(t, "enabled: true\nmodels:\n\tmisato: fable\n")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel, err := releaseconfig.Load(tt.repo(t))
			if !errors.Is(err, releaseconfig.ErrNotConfigured) {
				t.Fatalf("err = %v, want ErrNotConfigured", err)
			}
			if rel != nil {
				t.Fatalf("release = %+v, want nil", rel)
			}
		})
	}
}

// A strict-YAML error outside release: does not stop a valid section from
// loading, whether it comes before or after the section.
func TestLoad_ToleratesUnrelatedYAMLErrors(t *testing.T) {
	const section = "release:\n  projects:\n    - name: app\n      path: .\n"
	tests := []struct{ name, yaml string }{
		{"duplicate key before", "enabled: true\nenabled: false\n" + section},
		{"tab indentation after", section + "models:\n\tmisato: fable\n"},
		{"broken flow sequence before", "critical_paths: [auth/\n" + section},
		{"document start marker", "---\nenabled: true\n" + section},
		{"crlf line endings", strings.ReplaceAll("enabled: true\n"+section+"tasks:\n  provider: none\n", "\n", "\r\n")},
		{"section followed by a comment and a key", section + "# tasks\ntasks:\n  provider: none\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := mustLoad(t, tt.yaml)
			if len(rel.Projects) != 1 || rel.Projects[0].Name != "app" {
				t.Fatalf("projects = %+v, want [app]", rel.Projects)
			}
		})
	}
}

// A document whose top level is not a mapping is a broken .ai-workflow.yml, not a
// repository without release configuration.
func TestLoad_RootNotMapping(t *testing.T) {
	for _, text := range []string{"- a\n- b\n", "just a scalar\n"} {
		t.Run(strings.TrimSpace(text), func(t *testing.T) {
			_, err := releaseconfig.Load(writeConfig(t, text))
			var verr *releaseconfig.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("err = %v, want a *ValidationError", err)
			}
			if !strings.Contains(err.Error(), "top level of .ai-workflow.yml must be a mapping") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// Anchors and merge keys inside the section share settings between
// projects; an unknown key reached only through a merge is still reported.
func TestLoad_MergeKeys(t *testing.T) {
	rel := mustLoad(t, `release:
  projects:
    - &base
      name: api
      path: api
      build: bash
      version_files: [api/Api.csproj]
      hooks:
        prepare:
          - &bump { run: scripts/bump.sh, shell: pwsh }
    - <<: *base
      name: worker
      path: worker
      stages:
        test:
          <<: *bump
          run: scripts/test.sh
    - <<: [*base]
      name: cli
      path: cli
`)
	worker, cli := rel.Projects[1], rel.Projects[2]
	if worker.Build != "bash" || len(worker.VersionFiles) != 1 || worker.VersionFiles[0] != "api/Api.csproj" || worker.Path != "worker" {
		t.Errorf("worker = %+v", worker)
	}
	if got := worker.Stages["test"]; got.Run != "scripts/test.sh" || got.Shell != "pwsh" {
		t.Errorf("worker.stages.test = %+v", got)
	}
	if cli.Build != "bash" || cli.Path != "cli" {
		t.Errorf("cli = %+v", cli)
	}

	_, err := releaseconfig.Load(writeConfig(t, `release:
  projects:
    - name: api
      path: api
      x-shared: &shared { submodules: true, dockr: {} }
    - <<: *shared
      name: worker
      path: worker
`))
	if err == nil || !strings.Contains(err.Error(), "release.projects[1].dockr: unknown field") {
		t.Fatalf("err = %v, want the merged unknown key reported", err)
	}
}

// Anchors defined outside release: cannot be used inside it, because only
// the section is parsed.
func TestLoad_AliasToOutsideAnchor(t *testing.T) {
	_, err := releaseconfig.Load(writeConfig(t, `defaults: &cmd { run: scripts/test.sh }
release:
  projects:
    - name: app
      path: .
      stages:
        test: *cmd
`))
	var verr *releaseconfig.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want a *ValidationError", err)
	}
	for _, want := range []string{"*cmd", "line 7", "outside the release: section"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to contain %q", err, want)
		}
	}
}

// An alias whose anchor is defined nowhere before it (a typo, or an alias
// used before its anchor) is reported as undefined, not as an outside anchor.
func TestLoad_AliasToUndefinedAnchor(t *testing.T) {
	tests := []struct{ name, yaml, alias, line string }{
		{"typo", `release:
  projects:
    - name: app
      path: .
      hooks:
        prepare:
          - &bump { run: scripts/bump.sh }
      stages:
        test: *bmup
`, "*bmup", "line 9"},
		{"used before its definition", `release:
  projects:
    - name: app
      path: .
      stages:
        test: *cmd
      hooks:
        prepare:
          - &cmd { run: scripts/test.sh }
`, "*cmd", "line 6"},
		{"only a longer anchor name outside", `defaults: &cmdx { run: scripts/test.sh }
release:
  projects:
    - name: app
      path: .
      stages:
        test: *cmd
`, "*cmd", "line 7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := releaseconfig.Load(writeConfig(t, tt.yaml))
			var verr *releaseconfig.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("err = %v, want a *ValidationError", err)
			}
			for _, want := range []string{tt.alias, tt.line, "undefined anchor"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v, want it to contain %q", err, want)
				}
			}
			if strings.Contains(err.Error(), "outside") {
				t.Errorf("err = %v, want no mention of an outside anchor", err)
			}
		})
	}
}

// A top-level release key written with a node property or quoted is still
// found, even when the rest of the file is not valid YAML.
func TestLoad_ReleaseKeyForms(t *testing.T) {
	const body = "\n  projects:\n    - name: app\n      path: .\n"
	const broken = "models:\n\tmisato: fable\n"
	tests := []struct{ name, key string }{
		{"anchored", "&r release:"},
		{"tagged", "!!str release:"},
		{"anchored and tagged", "&r !!str release:"},
		{"verbatim tag", "!<tag:yaml.org,2002:str> release:"},
		{"double-quoted", `"release":`},
		{"single-quoted", "'release':"},
	}
	for _, tt := range tests {
		for _, layout := range []struct{ name, before string }{
			{"after another key", "enabled: true\n"},
			{"first key", ""},
		} {
			t.Run(tt.name+"/"+layout.name, func(t *testing.T) {
				rel := mustLoad(t, layout.before+tt.key+body+broken)
				if len(rel.Projects) != 1 || rel.Projects[0].Name != "app" {
					t.Fatalf("projects = %+v, want [app]", rel.Projects)
				}
			})
		}
	}
}

// A broken file with a line that may be a top-level release key the cut
// cannot recognise is an error, never a repository without configuration.
func TestLoad_UnrecognisedReleaseKeyInBrokenFile(t *testing.T) {
	_, err := releaseconfig.Load(writeConfig(t, `enabled: true
"s\x68ip":
  projects:
    - name: app
      path: .
models:
	misato: fable
`))
	if err == nil || errors.Is(err, releaseconfig.ErrNotConfigured) {
		t.Fatalf("err = %v, want a parse error", err)
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("err = %v, want it to name line 2", err)
	}
}

// Billion laughs through merge keys: the unknown-field walk has a budget and
// fails fast instead of expanding 10^9 nodes.
func TestLoad_AliasExpansionBudget(t *testing.T) {
	var b strings.Builder
	b.WriteString("release:\n  l0: &l0 { run: x }\n")
	for i := 1; i <= 9; i++ {
		fmt.Fprintf(&b, "  l%d: &l%d { <<: [", i, i)
		for j := range 10 {
			if j > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "*l%d", i-1)
		}
		b.WriteString("] }\n")
	}
	b.WriteString("  projects:\n    - name: a\n      path: .\n      stages:\n        test: *l9\n")
	repo := writeConfig(t, b.String())

	done := make(chan error, 1)
	go func() {
		_, err := releaseconfig.Load(repo)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "aliases and merge keys") {
			t.Fatalf("err = %v, want an alias expansion error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Load did not return within 10s")
	}
}

// An unknown key does not hide the other problems of the section.
func TestLoad_UnknownFieldsReportedWithOtherProblems(t *testing.T) {
	_, err := releaseconfig.Load(writeConfig(t, "release:\n  projects:\n    - name: api\n      pth: src\n      presets: [maven]\n      build: { mode: make }\n"))
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{
		"release.projects[0].pth: unknown field",
		"release.projects[0].path: required",
		"release.projects[0].presets: the presets were removed",
		"release.projects[0].build.mode: build is the build provider's name now",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestLoad_SingleProjectDefaults(t *testing.T) {
	rel := mustLoad(t, `enabled: true
critical_paths: [auth/]
release:
  projects:
    - name: app
      path: .
tasks:
  provider: none
`)
	if len(rel.Projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(rel.Projects))
	}
	p := rel.Projects[0]
	if p.Path != "." {
		t.Errorf("path = %q, want .", p.Path)
	}
	// The tag format and the changelog are the version engine's defaults
	// (internal/releaseplan's engine tests).
	if p.TagFormat != "" || p.Changelog != "" {
		t.Errorf("tag_format %q, changelog %q, want them left to the engine", p.TagFormat, p.Changelog)
	}
	if !reflect.DeepEqual(p.Scopes, []string{"app"}) {
		t.Errorf("scopes = %v, want [app]", p.Scopes)
	}
}

func TestLoad_MonorepoDefaults(t *testing.T) {
	rel := mustLoad(t, `release:
  projects:
    - name: api
      path: src\Api
      scopes: [api, backend]
      depends_on:
        - project: lib
    - name: lib
      path: ./src/Lib/
      tag_format: "lib@${version}"
      changelog: docs/LIB_CHANGELOG.md
`)
	api, lib := rel.Projects[0], rel.Projects[1]
	checks := []struct{ field, got, want string }{
		{"api.path", api.Path, "src/Api"},
		{"api.tag_format", api.TagFormat, ""},
		{"api.changelog", api.Changelog, ""},
		{"api.depends_on[0].bump", api.DependsOn[0].Bump, releaseconfig.DefaultBump},
		{"lib.path", lib.Path, "src/Lib"},
		{"lib.tag_format", lib.TagFormat, "lib@${version}"},
		{"lib.changelog", lib.Changelog, "docs/LIB_CHANGELOG.md"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}
	if !reflect.DeepEqual(api.Scopes, []string{"api", "backend"}) {
		t.Errorf("api.scopes = %v", api.Scopes)
	}
	if got, ok := rel.Project("lib"); !ok || got.Name != "lib" {
		t.Errorf("Project(lib) = %v, %v", got, ok)
	}
	if _, ok := rel.Project("nope"); ok {
		t.Errorf("Project(nope) found, want not found")
	}
}

// Stage commands and hooks get the default shell, and keep their env.
func TestLoad_CommandDefaults(t *testing.T) {
	rel := mustLoad(t, `release:
  projects:
    - name: api
      path: .
      build: bash
      stages:
        test: { run: "dotnet test --no-build" }
      hooks:
        prepare:
          - run: scripts/bump.sh
            env: { KEY: value }
          - run: scripts/bump.ps1
            shell: pwsh
`)
	p := rel.Projects[0]
	if got := p.Stages["test"]; got.Run != "dotnet test --no-build" || got.Shell != releaseconfig.DefaultShell {
		t.Errorf("stages.test = %+v", got)
	}
	hooks := p.Hooks["prepare"]
	if len(hooks) != 2 || hooks[0].Shell != releaseconfig.DefaultShell || hooks[0].Env["KEY"] != "value" || hooks[1].Shell != "pwsh" {
		t.Errorf("hooks.prepare = %+v", hooks)
	}
}

// workdir is normalised like every other path and timeout parses as a Go
// duration; both are optional.
func TestLoad_CommandWorkdirAndTimeout(t *testing.T) {
	rel := mustLoad(t, `release:
  projects:
    - name: api
      path: .
      stages:
        test: { run: "dotnet test", workdir: 'src\Api\', timeout: 10m }
      hooks:
        prepare:
          - run: scripts/bump.sh
          - run: build.ps1
            shell: pwsh
            workdir: ./tools/
            timeout: 1h30m
`)
	p := rel.Projects[0]
	if got := p.Stages["test"]; got.Workdir != "src/Api" || got.TimeoutDuration() != 10*time.Minute {
		t.Errorf("stages.test = %+v (timeout %s), want workdir src/Api and 10m", got, got.TimeoutDuration())
	}
	hooks := p.Hooks["prepare"]
	if hooks[0].Workdir != "" || hooks[0].TimeoutDuration() != 0 {
		t.Errorf("hooks.prepare[0] = %+v, want no workdir and no timeout", hooks[0])
	}
	if hooks[1].Workdir != "tools" || hooks[1].TimeoutDuration() != 90*time.Minute {
		t.Errorf("hooks.prepare[1] = %+v (timeout %s), want workdir tools and 1h30m", hooks[1], hooks[1].TimeoutDuration())
	}
}

// The first yaml example of docs/release-commands.md must stay loadable.
func TestLoad_DocumentedExample(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-commands.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(doc), "```yaml\n")
	example, _, closed := strings.Cut(rest, "```")
	if !ok || !closed {
		t.Fatal("docs/release-commands.md has no yaml example")
	}
	rel := mustLoad(t, example)
	p := rel.Projects[0]
	if p.Name != "api" || rel.BuildProvider(p) == "" {
		t.Errorf("documented example loaded as %+v", p)
	}
}

// The documented defaults are the documented values: a change to a default
// constant must be a deliberate change to docs/release-commands.md too.
func TestDefaults_MatchDocs(t *testing.T) {
	want := map[string]string{
		releaseconfig.DefaultBump:  "patch",
		releaseconfig.DefaultShell: "bash",
	}
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-commands.md"))
	if err != nil {
		t.Fatal(err)
	}
	for got, documented := range want {
		if got != documented {
			t.Errorf("default %q, documented as %q", got, documented)
		}
		if !strings.Contains(string(doc), "`"+documented+"`") {
			t.Errorf("docs/release-commands.md does not mention the default `%s`", documented)
		}
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	// one is a valid project prepended to cases that need a second project.
	const one = "release:\n  projects:\n    - name: lib\n      path: lib\n"
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"release not a mapping", "release: [a]\n", "release: must be a mapping"},
		{"unknown top-level release key", "release:\n  projets: []\n", `release.projets: unknown field`},
		{"unknown project key", one + "      dockr: {}\n", `release.projects[0].dockr: unknown field`},
		{"unknown nested key", one + "      coverage:\n        lin: 80\n", `release.projects[0].coverage.lin: unknown field`},
		{"wrong type", one + "      scopes: api\n", "release: line 5"},
		{"missing name", "release:\n  projects:\n    - path: .\n", `release.projects[0].name: required`},
		{"bad name chars", "release:\n  projects:\n    - name: My_Api\n      path: .\n", `release.projects[0].name: "My_Api" must match [a-z0-9][a-z0-9-]*`},
		{"duplicate name", one + "    - name: lib\n      path: other\n", `release.projects[1].name: duplicate project "lib"`},
		{"missing path", "release:\n  projects:\n    - name: api\n", `release.projects[0].path: required`},
		{"absolute path", "release:\n  projects:\n    - name: api\n      path: /src/api\n", `release.projects[0].path: "/src/api" must be relative to the repository root`},
		{"windows absolute path", "release:\n  projects:\n    - name: api\n      path: 'C:\\src'\n", `release.projects[0].path: "C:\\src" must be relative to the repository root`},
		{"escaping path", "release:\n  projects:\n    - name: api\n      path: src/../../x\n", `release.projects[0].path: "src/../../x" escapes the repository root`},
		{"escaping shared path", one + "      shared_paths: [../shared]\n", `release.projects[0].shared_paths[0]: "../shared" escapes the repository root`},
		{"escaping changelog", one + "      changelog: ../CHANGELOG.md\n", `release.projects[0].changelog: "../CHANGELOG.md" escapes the repository root`},
		{"empty scope", one + "      scopes: [\"\"]\n", `release.projects[0].scopes[0]: required`},
		{"scope with spaces", one + "      scopes: [\"my api\"]\n", `release.projects[0].scopes[0]: "my api" is not a valid conventional-commit scope`},
		{"tag format without version", one + "      tag_format: release-1\n", `release.projects[0].tag_format: "release-1" must contain ${version}`},
		{"tag format unknown placeholder", one + "      tag_format: \"${name}-${version}\"\n", `release.projects[0].tag_format: unknown placeholder ${name}`},
		{"duplicate tag format", "release:\n  projects:\n    - name: a\n      path: a\n      tag_format: v${version}\n    - name: b\n      path: b\n      tag_format: v${version}\n", `release.projects[1].tag_format: "v${version}" is already used by project "a"`},
		{"engine key in the section", "release:\n  tag_format: v1\n  projects:\n    - name: a\n      path: .\n", `release.tag_format: moved to release.providers.conventional-commits.tag_format`},
		{"unknown dependency", one + "      depends_on:\n        - project: lib2\n", `release.projects[0].depends_on[0].project: unknown project "lib2"`},
		{"missing dependency project", one + "      depends_on:\n        - bump: minor\n", `release.projects[0].depends_on[0].project: required`},
		{"self dependency", one + "      depends_on:\n        - project: lib\n", `release.projects[0].depends_on[0].project: a project cannot depend on itself`},
		{"duplicate dependency", one + "    - name: api\n      path: api\n      depends_on:\n        - project: lib\n        - project: lib\n", `release.projects[1].depends_on[1].project: duplicate dependency on "lib"`},
		{"dependency cycle", "release:\n  projects:\n    - name: a\n      path: a\n      depends_on: [{ project: b }]\n    - name: b\n      path: b\n      depends_on: [{ project: c }]\n    - name: c\n      path: c\n      depends_on: [{ project: a }]\n", `release.projects: dependency cycle a -> b -> c -> a`},
		{"unknown stage", one + "      stages:\n        deploy: { run: x }\n", `release.projects[0].stages.deploy: unknown stage (want restore, lint, build, test, coverage or package)`},
		{"stage without run", one + "      stages:\n        test: { shell: bash }\n", `release.projects[0].stages.test.run: required`},
		{"unknown hook", one + "      hooks:\n        before_deploy: [{ run: x }]\n", `release.projects[0].hooks.before_deploy: unknown hook`},
		{"unknown shell", one + "      hooks:\n        prepare: [{ run: x, shell: cmd }]\n", `release.projects[0].hooks.prepare[0].shell: unknown shell "cmd" (want node, pwsh or bash)`},
		{"hook without run", one + "      hooks:\n        after_test: [{ shell: bash }]\n", `release.projects[0].hooks.after_test[0].run: required`},
		{"reserved hook env", one + "      hooks:\n        fail: [{ run: x, env: { NERV_RELEASE_VERSION: \"1\" } }]\n", `release.projects[0].hooks.fail[0].env.NERV_RELEASE_VERSION: the NERV_RELEASE_ prefix is reserved`},
		{"escaping stage workdir", one + "      stages:\n        test: { run: x, workdir: ../up }\n", `release.projects[0].stages.test.workdir: "../up" escapes the repository root`},
		{"absolute hook workdir", one + "      hooks:\n        prepare: [{ run: x, workdir: /tmp }]\n", `release.projects[0].hooks.prepare[0].workdir: "/tmp" must be relative to the repository root`},
		{"timeout not a duration", one + "      stages:\n        test: { run: x, timeout: soon }\n", `release.projects[0].stages.test.timeout: "soon" is not a duration such as 90s or 10m`},
		{"timeout without unit", one + "      hooks:\n        fail: [{ run: x, timeout: 90 }]\n", `release.projects[0].hooks.fail[0].timeout: "90" is not a duration such as 90s or 10m`},
		{"zero timeout", one + "      stages:\n        build: { run: x, timeout: 0s }\n", `release.projects[0].stages.build.timeout: "0s" must be positive`},
		{"negative timeout", one + "      stages:\n        build: { run: x, timeout: -5m }\n", `release.projects[0].stages.build.timeout: "-5m" must be positive`},
		{"unknown bump", one + "    - name: a\n      path: a\n      depends_on: [{ project: lib, bump: huge }]\n", `release.projects[1].depends_on[0].bump: unknown bump "huge" (want patch, minor, major or none)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel, err := releaseconfig.Load(writeConfig(t, tt.yaml))
			if err == nil {
				t.Fatalf("Load succeeded with %+v, want error containing %q", rel, tt.want)
			}
			if errors.Is(err, releaseconfig.ErrNotConfigured) {
				t.Fatalf("err = ErrNotConfigured, want a validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error:\n%v\nwant it to contain:\n%s", err, tt.want)
			}
		})
	}
}

// Every problem is reported at once, so a user fixes the file in one pass.
func TestLoad_ReportsAllProblems(t *testing.T) {
	_, err := releaseconfig.Load(writeConfig(t, "release:\n  projects:\n    - name: Bad\n    - path: /x\n"))
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{
		"release.projects[0].name:",
		"release.projects[0].path: required",
		"release.projects[1].name: required",
		"release.projects[1].path:",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func names(ps []releaseconfig.Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}

func TestTopoOrder(t *testing.T) {
	tests := []struct {
		name     string
		projects []releaseconfig.Project
		want     []string
		wantErr  string
	}{
		{
			name:     "independent projects keep declaration order",
			projects: []releaseconfig.Project{{Name: "b"}, {Name: "a"}},
			want:     []string{"b", "a"},
		},
		{
			name: "dependencies come first",
			projects: []releaseconfig.Project{
				{Name: "app", DependsOn: []releaseconfig.Dependency{{Project: "lib"}}},
				{Name: "tool", DependsOn: []releaseconfig.Dependency{{Project: "app"}}},
				{Name: "lib"},
			},
			want: []string{"lib", "app", "tool"},
		},
		{
			name: "diamond",
			projects: []releaseconfig.Project{
				{Name: "top", DependsOn: []releaseconfig.Dependency{{Project: "left"}, {Project: "right"}}},
				{Name: "left", DependsOn: []releaseconfig.Dependency{{Project: "base"}}},
				{Name: "right", DependsOn: []releaseconfig.Dependency{{Project: "base"}}},
				{Name: "base"},
			},
			want: []string{"base", "left", "right", "top"},
		},
		{
			name: "two-node cycle",
			projects: []releaseconfig.Project{
				{Name: "x"},
				{Name: "a", DependsOn: []releaseconfig.Dependency{{Project: "b"}}},
				{Name: "b", DependsOn: []releaseconfig.Dependency{{Project: "a"}}},
			},
			wantErr: "dependency cycle a -> b -> a",
		},
		{
			name:     "self cycle",
			projects: []releaseconfig.Project{{Name: "a", DependsOn: []releaseconfig.Dependency{{Project: "a"}}}},
			wantErr:  "dependency cycle a -> a",
		},
		{
			name:     "unknown dependency",
			projects: []releaseconfig.Project{{Name: "a", DependsOn: []releaseconfig.Dependency{{Project: "ghost"}}}},
			wantErr:  `unknown project "ghost"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := &releaseconfig.Release{Projects: tt.projects}
			got, err := rel.TopoOrder()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(names(got), tt.want) {
				t.Fatalf("order = %v, want %v", names(got), tt.want)
			}
		})
	}
}
