package migrate_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployprovider"
	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

const (
	comprobantes = "dim.comprobantes.api"
	factura      = "dim-factura-electronica-service"
	legacyFull   = "legacy-full"
	githubOrigin = "https://github.com/dirm-s-a/repo.git"
)

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

// world is a repository with a 4.2.0 configuration and a home.
type world struct {
	t          *testing.T
	root, home string
}

// newWorld copies testdata/<fixture> into a fresh repository: its
// nerv.yaml becomes .nerv/nerv.yaml, every other file keeps its path.
func newWorld(t *testing.T, fixture string) world {
	t.Helper()
	w := world{t: t, root: filepath.Join(t.TempDir(), "repo"), home: t.TempDir()}
	src := filepath.Join("testdata", fixture)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if rel == "nerv.yaml" {
			rel = filepath.Join(".nerv", "nerv.yaml")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		w.write(rel, string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func (w world) write(rel, content string) {
	w.t.Helper()
	path := filepath.Join(w.root, filepath.FromSlash(rel))
	if filepath.IsAbs(rel) {
		path = rel
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w world) read(rel string) string {
	w.t.Helper()
	path := filepath.Join(w.root, filepath.FromSlash(rel))
	if filepath.IsAbs(rel) {
		path = rel
	}
	data, err := os.ReadFile(path)
	if err != nil {
		w.t.Fatal(err)
	}
	return string(data)
}

func (w world) exists(rel string) bool {
	path := filepath.Join(w.root, filepath.FromSlash(rel))
	if filepath.IsAbs(rel) {
		path = rel
	}
	_, err := os.Lstat(path)
	return err == nil
}

func (w world) options(user bool, origin string) migrate.Options {
	return migrate.Options{RepoRoot: w.root, Home: w.home, User: user, ExtensionsFS: nerv.ExtensionsFS(), OriginURL: origin, Now: now}
}

func (w world) plan(o migrate.Options) *migrate.Plan {
	w.t.Helper()
	p, err := migrate.Compute(o)
	if err != nil {
		w.t.Fatalf("Compute: %v", err)
	}
	return p
}

func (w world) migrate(o migrate.Options) (*migrate.Plan, []string) {
	w.t.Helper()
	p := w.plan(o)
	backups, err := migrate.Apply(p, o.Now)
	if err != nil {
		w.t.Fatalf("Apply: %v", err)
	}
	return p, backups
}

func (w world) userPaths() paths.Paths { return paths.Resolve(w.home) }

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

func anyContains(lines []string, s string) bool {
	return slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, s) })
}

// treeHash hashes every file under dirs (path and content).
func treeHash(t *testing.T, dirs ...string) string {
	t.Helper()
	h := sha256.New()
	for _, dir := range dirs {
		var files []string
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				files = append(files, path)
			}
			return nil
		})
		sort.Strings(files)
		for _, f := range files {
			data, _ := os.ReadFile(f)
			h.Write([]byte(f + "\x00"))
			h.Write(data)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Rule 1: .nerv/nerv.yaml moves to .ai-workflow.yml with its comments and
// key order, and the old file stays as .nerv/nerv.yaml.migrated.
func TestConfig_ProjectFileMovesKeepingCommentsAndOrder(t *testing.T) {
	w := newWorld(t, comprobantes)
	old := w.read(".nerv/nerv.yaml")
	w.migrate(w.options(false, githubOrigin))

	if w.exists(".nerv/nerv.yaml") {
		t.Error(".nerv/nerv.yaml still exists: an old nerv would read stale configuration")
	}
	if got := w.read(".nerv/nerv.yaml.migrated"); got != old {
		t.Error(".nerv/nerv.yaml.migrated is not the old file")
	}
	text := w.read(".ai-workflow.yml")
	prefix := old[:strings.Index(old, "release:")]
	if !strings.HasPrefix(text, prefix) {
		t.Errorf(".ai-workflow.yml does not keep the old file's head byte for byte:\n%s", text)
	}
	contains(t, ".ai-workflow.yml", text,
		"# No build servers: the shared T2 host aborts dotnet with",
		// yaml.v3 writes a flow mapping without the inner spaces.
		`env: {MSBUILDDISABLENODEREUSE: "1", UseSharedCompilation: "false", DOTNET_CLI_USE_MSBUILD_SERVER: "0"}`)
	if strings.Index(text, "git:") > strings.Index(text, "tasks:") || strings.Index(text, "tasks:") > strings.Index(text, "ship:") {
		t.Errorf("the key order changed:\n%s", text)
	}
}

// Rule 1: the old user file moves to the user config path, its header
// rewritten, and stays as .migrated.
func TestConfig_UserFileMovesToTheUserConfig(t *testing.T) {
	w := newWorld(t, comprobantes)
	up := w.userPaths()
	w.write(up.LegacyUserConfig, "# ~/.claude/nerv/nerv.yaml -- NERV user-scope configuration\ngit:\n  worktree: ask\nmodels:\n  misato: { model: fable, effort: max }\n")
	w.migrate(migrate.Options{Home: w.home, User: true, ExtensionsFS: nerv.ExtensionsFS(), Now: now})

	got := w.read(up.UserConfig)
	want := "# the user config.yml -- NERV user-scope configuration\ngit:\n  worktree: ask\nmodels:\n  misato: { model: fable, effort: max }\n"
	if got != want {
		t.Errorf("user config:\n%s\nwant:\n%s", got, want)
	}
	if w.exists(up.LegacyUserConfig) || !w.exists(up.LegacyUserConfig+migrate.MigratedSuffix) {
		t.Error("the old user file was not renamed to .migrated")
	}
	if w.exists(".ai-workflow.yml") {
		t.Error("--user without a repository migrated the repository")
	}
}

// Rule 1: personal keys of the project file are reported without --user.
func TestPersonalKeys_ReportedWithoutUser(t *testing.T) {
	w := newWorld(t, legacyFull)
	p, _ := w.migrate(w.options(false, githubOrigin))
	text := w.read(".ai-workflow.yml")
	contains(t, ".ai-workflow.yml", text, "models:\n  misato: { model: fable, effort: high }", "assignee_id: 42")
	for _, want := range []string{"models: personal settings", "tasks.providers.teamwork.assignee_id: a personal setting", "config.infisical_domain: a personal setting"} {
		if !anyContains(p.Warnings, want) {
			t.Errorf("no warning %q in %q", want, p.Warnings)
		}
	}
	if w.exists(w.userPaths().UserConfig) {
		t.Error("the user config was written without --user")
	}
}

// Rule 1: with --user in the same run the personal keys move to the user
// config, and the project file keeps only repository settings.
func TestPersonalKeys_MoveWithUser(t *testing.T) {
	w := newWorld(t, legacyFull)
	w.migrate(w.options(true, githubOrigin))
	project := w.read(".ai-workflow.yml")
	user := w.read(w.userPaths().UserConfig)
	lacks(t, ".ai-workflow.yml", project, "models:", "assignee_id", "infisical_domain", "ssh_auth", "ssh_key_file")
	contains(t, "user config", user,
		"models:\n  misato: { model: fable, effort: high }",
		"tasks:\n  providers:\n    teamwork:\n      assignee_id: 42",
		"deploy:\n  providers:\n    ssh-compose:\n",
		"infisical_domain: \"https://infisical.acme\"", "ssh_auth: key", "ssh_key_file: ~/.ssh/id_deploy")
	contains(t, ".ai-workflow.yml", project, "project_id: 100", "tasklist_id: 200")
}

// Rule 2: tasks.rounding_minutes moves to the teamwork provider.
func TestTasks_RoundingMinutesMovesToTeamwork(t *testing.T) {
	w := newWorld(t, legacyFull)
	w.migrate(w.options(false, githubOrigin))
	doc := config.Parse(w.read(".ai-workflow.yml"))
	if v, ok := doc.Scalar("tasks.providers.teamwork.rounding_minutes"); !ok || v != "30" {
		t.Errorf("tasks.providers.teamwork.rounding_minutes = %q, %v; want 30", v, ok)
	}
	if _, ok := doc.Scalar("tasks.rounding_minutes"); ok {
		t.Error("tasks.rounding_minutes is still there")
	}
}

// Rule 2: the removed providers (jira, github-projects) are dropped with a
// report line, as is teamwork's task_ref_prefix.
func TestTasks_RemovedProvidersAreDropped(t *testing.T) {
	w := newWorld(t, legacyFull)
	w.write(".nerv/nerv.yaml", strings.Replace(w.read(".nerv/nerv.yaml"), "  provider: teamwork\n", "  provider: github-projects\n", 1))
	p, _ := w.migrate(w.options(false, githubOrigin))
	text := w.read(".ai-workflow.yml")
	lacks(t, ".ai-workflow.yml", text, "jira", "github-projects", "task_ref_prefix")
	for _, want := range []string{"tasks.provider: github-projects was removed", "tasks.providers.jira: the jira provider was removed"} {
		if !anyContains(p.Warnings, want) {
			t.Errorf("no warning %q in %q", want, p.Warnings)
		}
	}
	if !anyContains(p.Notes, "task_ref_prefix dropped") {
		t.Errorf("no note on task_ref_prefix in %q", p.Notes)
	}
}

// Rule 2: the migrated task settings load with the task providers' strict
// rules: every key is declared, its value has the declared type, and the
// project file holds only repository-scope keys once --user moved the rest.
func TestTasks_MigratedSettingsLoadWithTheProviderManifests(t *testing.T) {
	providers, err := configure.EmbeddedProviders(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []string{comprobantes, factura, legacyFull} {
		t.Run(fixture, func(t *testing.T) {
			w := newWorld(t, fixture)
			w.migrate(w.options(true, githubOrigin))
			check := func(file, scope string) {
				text := w.read(file)
				for _, pr := range providers {
					for _, c := range pr.Manifest.Config {
						v, ok := config.Parse(text).Scalar(configure.ProviderKeyPath(pr.Manifest.Name, c.Key))
						if !ok {
							continue
						}
						if scope == "repo" && c.Scope != "repo" {
							t.Errorf("%s: %s is a %s-scope key", file, c.Key, c.Scope)
						}
						if c.Scalar() {
							if err := configure.ValidateProviderValue(pr.Manifest.Name, c, v); err != nil {
								t.Errorf("%s: %v", file, err)
							}
						}
					}
				}
			}
			check(".ai-workflow.yml", "repo")
			if w.exists(w.userPaths().UserConfig) {
				check(w.userPaths().UserConfig, "user")
			}
		})
	}
}

// Rule 3: release: becomes ship:, the presets become build.mode: script
// with the commands 4.2.0 ran, the project's own stage commands win, and
// the result loads with today's strict loader.
func TestShip_ReleaseBecomesScriptModeShip(t *testing.T) {
	w := newWorld(t, comprobantes)
	old := w.read(".nerv/nerv.yaml")
	w.migrate(w.options(false, githubOrigin))
	rel, err := shipconfig.Load(w.root)
	if err != nil {
		t.Fatalf("shipconfig.Load: %v", err)
	}
	if err := rel.CheckBuild(); err != nil {
		t.Fatalf("CheckBuild: %v", err)
	}
	api, _ := rel.Project("api")
	if api.Build == nil || api.Build.Mode != shipconfig.ModeScript {
		t.Fatalf("build = %+v, want mode script", api.Build)
	}
	if !slices.Equal(api.Build.VersionFiles, []string{"src/Api/Dim.Comprobantes.Api.csproj"}) {
		t.Errorf("version_files = %q, want the dotnet project_file", api.Build.VersionFiles)
	}
	for _, stage := range []string{"restore", "lint", "build", "test", "coverage"} {
		if !strings.Contains(old, "run: "+api.Stages[stage].Run) {
			t.Errorf("stages.%s = %q, not the project's own 4.2.0 command", stage, api.Stages[stage].Run)
		}
	}
	pkg := api.Stages["package"].Run
	contains(t, "stages.package", pkg,
		"docker build -f docker/Dockerfile -t ghcr.io/dirm-s-a/dim.comprobantes.api:\"$NERV_RELEASE_VERSION\"",
		`docker save -o "$NERV_RELEASE_OUTPUTS_DIR/dim.comprobantes.api.tar"`,
		`docker save -o "$NERV_RELEASE_OUTPUTS_DIR/dim.comprobantes.api-login-broker.tar"`)
	text := w.read(".ai-workflow.yml")
	contains(t, ".ai-workflow.yml", text, "# "+migrate.TODOPrefix+" 4.2.0 pushed ghcr.io/dirm-s-a/dim.comprobantes.api, ghcr.io/dirm-s-a/dim.comprobantes.api-login-broker")
	lacks(t, ".ai-workflow.yml", text, "\nrelease:", "presets:", "semantic_release:")
}

// Rule 3: the presets without an equivalent (nuget, npm push) get a TODO
// comment and a report line; the dotnet preset's commands and coverage
// threshold, node's install and scripts, and the hooks move over.
func TestShip_PresetsWithoutAnEquivalentGetATODO(t *testing.T) {
	w := newWorld(t, legacyFull)
	p, _ := w.migrate(w.options(false, githubOrigin))
	rel, err := shipconfig.Load(w.root)
	if err != nil {
		t.Fatalf("shipconfig.Load: %v", err)
	}
	web, _ := rel.Project("web")
	if got := web.Stages["restore"]; got.Run != "pnpm install --frozen-lockfile" || got.Workdir != "web" {
		t.Errorf("web restore = %+v", got)
	}
	if web.Stages["build"].Run != "pnpm run build" || web.Stages["test"].Run != "pnpm run test" {
		t.Errorf("web build/test = %q / %q", web.Stages["build"].Run, web.Stages["test"].Run)
	}
	if _, ok := web.Stages["lint"]; ok {
		t.Error("web has a lint stage, but package.json has no lint script")
	}
	lib, _ := rel.Project("lib")
	if lib.Stages["lint"].Run != "dotnet format lib/Lib.csproj --verify-no-changes --no-restore" {
		t.Errorf("lib lint = %q, want the project's own", lib.Stages["lint"].Run)
	}
	if lib.Stages["build"].Run != `dotnet build lib/Lib.csproj -c Release --no-restore --disable-build-servers -p:Version="$NERV_RELEASE_VERSION"` {
		t.Errorf("lib build = %q", lib.Stages["build"].Run)
	}
	contains(t, "lib test", lib.Stages["test"].Run, `--collect "XPlat Code Coverage" --results-directory "$NERV_RELEASE_ARTIFACTS_DIR/coverage"`)
	if lib.Build.Coverage == nil || lib.Build.Coverage.Line != 80 {
		t.Errorf("lib coverage = %+v, want line 80", lib.Build.Coverage)
	}
	if lib.Stages["build"].Env["MSBUILDDISABLENODEREUSE"] != "1" {
		t.Errorf("lib build env = %v, want the build-server-safe environment", lib.Stages["build"].Env)
	}
	contains(t, "lib package", lib.Stages["package"].Run, "dotnet pack lib/Lib.csproj", "docker build -f lib/Dockerfile", `--build-arg CHANNEL="$channel_tag"`, `--build-arg TAG=v-"$NERV_RELEASE_TAG"`)
	if len(lib.Hooks["before_build"]) != 1 {
		t.Errorf("hooks.before_build = %v, want kept", lib.Hooks["before_build"])
	}
	text := w.read(".ai-workflow.yml")
	contains(t, ".ai-workflow.yml", text,
		migrate.TODOPrefix+" 4.2.0 pushed the packages to https://api.nuget.org/v3/index.json",
		migrate.TODOPrefix+` 4.2.0 ran "npm publish --access restricted"`,
		migrate.TODOPrefix+" 4.2.0 dropped --disable-build-servers")
	for _, want := range []string{"stages.publish dropped", "hooks.success dropped", "nuget push", "npm publish has no deploy provider"} {
		if !anyContains(p.Warnings, want) {
			t.Errorf("no warning %q in %q", want, p.Warnings)
		}
	}
}

// Rule 3: the semantic-release configuration is imported into the ship:
// section through versioning.Import, with its warnings reported.
func TestShip_ImportsTheSemanticReleaseConfiguration(t *testing.T) {
	w := newWorld(t, comprobantes)
	p, _ := w.migrate(w.options(false, githubOrigin))
	rel, err := shipconfig.Load(w.root)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := versioning.Import(w.root)
	if err != nil {
		t.Fatal(err)
	}
	got := rel.Versioning()
	if len(got.Branches) != len(imported.Config.Branches) {
		t.Fatalf("branches = %+v, want the imported %+v", got.Branches, imported.Config.Branches)
	}
	for i := range got.Branches {
		if got.Branches[i].Name != imported.Config.Branches[i].Name || got.Branches[i].Channel != imported.Config.Branches[i].Channel {
			t.Errorf("branch %d = %+v, want %+v", i, got.Branches[i], imported.Config.Branches[i])
		}
	}
	if got.Changelog.File != "src/Api/CHANGELOG.md" {
		t.Errorf("changelog.file = %q, want the imported src/Api/CHANGELOG.md", got.Changelog.File)
	}
	for _, want := range []string{"@semantic-release/exec", "@semantic-release/npm", "@semantic-release/github"} {
		if !anyContains(p.Warnings, want) {
			t.Errorf("no import warning for %s in %q", want, p.Warnings)
		}
	}
}

// Rule 4: pipeline: becomes deploy-ssh-compose targets plus the
// deploy-github host, loadable by deployconfig and the provider manifest;
// what has no equivalent is reported.
func TestDeploy_PipelineBecomesTargetsAndHost(t *testing.T) {
	w := newWorld(t, legacyFull)
	p, _ := w.migrate(w.options(false, githubOrigin))
	d, err := deployconfig.Load(w.root)
	if err != nil {
		t.Fatalf("deployconfig.Load: %v", err)
	}
	if d.Host != "github" {
		t.Errorf("host = %q, want github", d.Host)
	}
	data, err := fs.ReadFile(nerv.ExtensionsFS(), "deploy-ssh-compose/deploy-provider.yaml")
	if err != nil {
		t.Fatal(err)
	}
	m, err := deployprovider.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tg := range d.Targets {
		names = append(names, tg.Name)
		if tg.Provider != "ssh-compose" {
			t.Errorf("%s provider = %q", tg.Name, tg.Provider)
		}
		for k, v := range tg.Config {
			c, ok := m.Key(k)
			if !ok {
				t.Errorf("%s: config.%s is not a key of ssh-compose", tg.Name, k)
				continue
			}
			if !deployprovider.ValidValue(c.Type, v) {
				t.Errorf("%s: config.%s = %q is not a valid %s", tg.Name, k, v, c.Type)
			}
		}
		want := map[string]string{"environment": tg.Name, "source": "infisical", "infisical_project_id": "p-1", "infisical_app_path": "/app", "ssh_port": "2222", "verify_timeout": "300s"}
		for k, v := range want {
			if tg.Config[k] != v {
				t.Errorf("%s: config.%s = %q, want %q", tg.Name, k, tg.Config[k], v)
			}
		}
	}
	if !slices.Equal(names, []string{"staging", "production"}) {
		t.Errorf("targets = %q, want staging, production in order", names)
	}
	for _, want := range []string{"selection: branches has no equivalent", "pipeline.sync", "pipeline.registry", "pipeline.dotnet", "pipeline.images"} {
		if !anyContains(p.Warnings, want) {
			t.Errorf("no warning %q in %q", want, p.Warnings)
		}
	}
	lacks(t, ".ai-workflow.yml", w.read(".ai-workflow.yml"), "pipeline:")
}

// Rule 4: a repository whose semantic-release published GitHub releases
// gets deploy.host github when origin is GitHub, and a report otherwise.
func TestDeploy_GitHubHostFollowsOrigin(t *testing.T) {
	w := newWorld(t, comprobantes)
	w.migrate(w.options(false, githubOrigin))
	d, err := deployconfig.Load(w.root)
	if err != nil || d.Host != "github" {
		t.Fatalf("deploy = %+v, %v; want host github", d, err)
	}

	w = newWorld(t, comprobantes)
	p, _ := w.migrate(w.options(false, "https://gitlab.com/acme/repo.git"))
	d, err = deployconfig.Load(w.root)
	if err != nil || d.Host != "" {
		t.Fatalf("deploy = %+v, %v; want no host for a GitLab origin", d, err)
	}
	if !anyContains(p.Warnings, "is not GitHub") {
		t.Errorf("no warning about the origin in %q", p.Warnings)
	}
}

// openspecFixture is an in-flight change, an archived one and the specs.
func (w world) openspecFixture(strict bool) {
	cfg := "context: |\n  a repo\nstrict_tdd: false\ntesting:\n  runner: none\n"
	if strict {
		cfg = "context: |\n  a repo\nstrict_tdd: true\ntesting:\n  runner: dotnet test App.slnx\n"
	}
	w.write("openspec/config.yaml", cfg)
	c := "openspec/changes/tw-1-feature/"
	w.write(c+"state.yaml", "dependsOn: []\nnerv:\n  path: full\n  task_ref: \"TW-1\"\n  branch: feature/tw-1-feature\n  created_at: \"2026-10-23T21:58:38Z\"\n")
	w.write(c+"proposal.md", "# Proposal: tw-1-feature\n\n## Intent\n\n> \"Haceme X\"\n")
	w.write(c+"design.md", "# Design\n\n## Decisions\n\n- D1: do it\n")
	w.write(c+"tasks.md", "# Tasks\n\n## T1. First task\n\n- id: T1\n- pilot: shinji\n- depends_on: []\n- status: planned\n- specs: S1, S2\n\n## T2. Second\n\n- id: T2\n- pilot: rei\n- depends_on: [T1]\n- status: done\n- specs: S3\n")
	w.write(c+"specs/receipts/spec.md", "# Receipts\n\n### S1. A rule\n")
	w.write(c+"exploration.md", "explored\n")
	w.write(c+"nerv/deliberation-log.md", "log\n")
	w.write(c+"nerv/test-plan.md", "plan\n")
	w.write(c+"nerv/.orchestrator.lock", "session_id: abc\n")
	w.write("openspec/changes/archive/2026-01-01-old/proposal.md", "old\n")
	w.write("openspec/specs/receipts/spec.md", "spec\n")
}

// Rule 5: an in-flight openspec change becomes an ODD feature document,
// its run files move to odd/<change>/nerv/ and its lock to
// .nerv/locks/<change>.lock; archived changes and openspec/specs stay.
func TestODD_InFlightChangeBecomesAFeatureDocument(t *testing.T) {
	w := newWorld(t, comprobantes)
	w.openspecFixture(true)
	p, _ := w.migrate(w.options(false, githubOrigin))

	doc := w.read("odd/tasks/tw-1-feature.md")
	contains(t, "the ODD document", doc,
		"# tw-1-feature\n", "Branch: `feature/tw-1-feature`", "task TW-1", "Test runner: dotnet test App.slnx (strict TDD).",
		"## Specs\n", "### Receipts", "#### S1. A rule", "### Proposal: tw-1-feature", `> "Haceme X"`,
		"## Tasks\n", "| T1 | S1, S2 | delegated (FULL) | shinji | — | — | — | pending | — |", "| T2 | S3 | delegated (FULL) | rei | T1 | — | — | done | — |",
		"- **T1:** First task",
		"## Log\n", "- **L1** ", "design (from `design.md`)", "#### D", "exploration: `nerv/exploration.md`", "test plan: `nerv/test-plan.md`")
	if strings.Index(doc, "## Specs") > strings.Index(doc, "## Tasks") || strings.Index(doc, "## Tasks") > strings.Index(doc, "## Log") {
		t.Error("the document's sections are out of order")
	}
	for rel, want := range map[string]string{
		"odd/tw-1-feature/nerv/deliberation-log.md": "log\n",
		"odd/tw-1-feature/nerv/test-plan.md":        "plan\n",
		"odd/tw-1-feature/nerv/state.yaml":          "dependsOn: []\n",
		"odd/tw-1-feature/nerv/exploration.md":      "explored\n",
		".nerv/locks/tw-1-feature.lock":             "session_id: abc\n",
	} {
		if !w.exists(rel) || !strings.HasPrefix(w.read(rel), want) {
			t.Errorf("%s is missing or wrong", rel)
		}
	}
	if w.exists("openspec/changes/tw-1-feature") {
		t.Error("the in-flight change folder is still there")
	}
	for _, rel := range []string{"openspec/changes/archive/2026-01-01-old/proposal.md", "openspec/specs/receipts/spec.md", "openspec/config.yaml"} {
		if !w.exists(rel) {
			t.Errorf("%s was removed; it must stay", rel)
		}
	}
	for _, want := range []string{"1 archived change(s) stay", "openspec/specs stays"} {
		if !anyContains(p.Notes, want) {
			t.Errorf("no note %q in %q", want, p.Notes)
		}
	}
}

// Rule 5: openspec/config.yaml's strict_tdd becomes the default TDD mode
// the orchestrator reads, odd.tdd and odd.test_runner.
func TestODD_StrictTDDBecomesTheDefaultMode(t *testing.T) {
	for _, strict := range []bool{true, false} {
		w := newWorld(t, comprobantes)
		w.openspecFixture(strict)
		w.migrate(w.options(false, githubOrigin))
		doc := config.Parse(w.read(".ai-workflow.yml"))
		mode, _ := doc.Scalar("odd.tdd")
		runner, _ := doc.Scalar("odd.test_runner")
		wantMode, wantRunner := "standard", "none"
		if strict {
			wantMode, wantRunner = "strict", "dotnet test App.slnx"
		}
		if mode != wantMode || runner != wantRunner {
			t.Errorf("strict_tdd %v: odd.tdd = %q, odd.test_runner = %q; want %q, %q", strict, mode, runner, wantMode, wantRunner)
		}
	}
}

// Rule 6: the plugins the old configuration used are named in one nerv
// install line, in install order.
func TestPlugins_InstallLineNamesWhatTheRepositoryUsed(t *testing.T) {
	cases := []struct {
		fixture, origin, want string
	}{
		{comprobantes, githubOrigin, "nerv install --with task,task-teamwork,ship,deploy,deploy-github"},
		{legacyFull, githubOrigin, "nerv install --with task,task-teamwork,ship,deploy,deploy-github,deploy-ssh-compose"},
		{comprobantes, "", "nerv install --with task,task-teamwork,ship,deploy"},
	}
	for _, c := range cases {
		w := newWorld(t, c.fixture)
		if got := w.plan(w.options(false, c.origin)).InstallCommand(); got != c.want {
			t.Errorf("%s (origin %q): %q, want %q", c.fixture, c.origin, got, c.want)
		}
	}
	w := newWorld(t, comprobantes)
	w.write(".nerv/nerv.yaml", "enabled: true\ntasks:\n  provider: none\n")
	if got := w.plan(w.options(false, githubOrigin)).InstallCommand(); got != "" {
		t.Errorf("provider none, no release: %q, want no install line", got)
	}

	// Without a project provider, the user's decides (new file first, then
	// the old one), and with none at all the 4.2.0 default, teamwork.
	w.write(".nerv/nerv.yaml", "enabled: true\n")
	if got := w.plan(w.options(false, githubOrigin)).InstallCommand(); got != "nerv install --with task,task-teamwork" {
		t.Errorf("no provider anywhere: %q, want the 4.2.0 default teamwork", got)
	}
	w.write(w.userPaths().LegacyUserConfig, "tasks:\n  provider: none\n")
	if got := w.plan(w.options(false, githubOrigin)).InstallCommand(); got != "" {
		t.Errorf("old user file says none: %q, want no install line", got)
	}
	w.write(w.userPaths().UserConfig, "tasks:\n  provider: teamwork\n")
	if got := w.plan(w.options(false, githubOrigin)).InstallCommand(); got != "nerv install --with task,task-teamwork" {
		t.Errorf("new user config says teamwork: %q", got)
	}
}

// Rule 7: the runtime folder gets its .gitignore of "*".
func TestRuntime_GitignoreIsCreated(t *testing.T) {
	w := newWorld(t, comprobantes)
	w.migrate(w.options(false, githubOrigin))
	if got := w.read(".nerv/.gitignore"); got != "*\n" {
		t.Errorf(".nerv/.gitignore = %q, want \"*\\n\"", got)
	}
}

// --dry-run writes nothing: Compute reads only.
func TestDryRun_WritesNothing(t *testing.T) {
	w := newWorld(t, legacyFull)
	w.openspecFixture(true)
	w.write(w.userPaths().LegacyUserConfig, "tasks:\n  rounding_minutes: 30\n")
	before := treeHash(t, w.root, w.home)
	p := w.plan(w.options(true, githubOrigin))
	if p.Empty() {
		t.Fatal("the plan is empty")
	}
	diff := migrate.Render(p)
	contains(t, "the rendered plan", diff, "--- /dev/null\n+++ b/.ai-workflow.yml\n", "move .nerv/nerv.yaml -> .nerv/nerv.yaml.migrated", "move openspec/changes/tw-1-feature/nerv/test-plan.md -> odd/tw-1-feature/nerv/test-plan.md", "remove openspec/changes/tw-1-feature/proposal.md", "+ship:")
	if after := treeHash(t, w.root, w.home); after != before {
		t.Error("planning changed the files")
	}
}

// A second run changes nothing.
func TestSecondRun_IsANoOp(t *testing.T) {
	for _, fixture := range []string{comprobantes, factura, legacyFull} {
		w := newWorld(t, fixture)
		w.openspecFixture(true)
		w.write(w.userPaths().LegacyUserConfig, "tasks:\n  rounding_minutes: 30\n")
		w.migrate(w.options(true, githubOrigin))
		before := treeHash(t, w.root, w.home)
		p := w.plan(w.options(true, githubOrigin))
		if !p.Empty() {
			t.Errorf("%s: the second plan has changes:\n%s", fixture, migrate.Render(p))
		}
		if _, err := migrate.Apply(p, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if treeHash(t, w.root, w.home) != before {
			t.Errorf("%s: the second run changed files", fixture)
		}
	}
}

// Every changed or removed file is backed up first.
func TestBackups_EveryChangedOrRemovedFile(t *testing.T) {
	w := newWorld(t, comprobantes)
	w.openspecFixture(true)
	up := w.userPaths()
	w.write(up.LegacyUserConfig, "tasks:\n  rounding_minutes: 30\n")
	oldProject := w.read(".nerv/nerv.yaml")
	_, backups := w.migrate(w.options(true, githubOrigin))
	stamp := now.Format("20060102-150405")
	backupDir := filepath.Join(w.root, ".nerv", "backups", "migrate-"+stamp)
	for rel, want := range map[string]string{
		".nerv/nerv.yaml":                                       oldProject,
		"openspec/changes/tw-1-feature/tasks.md":                "# Tasks",
		"openspec/changes/tw-1-feature/nerv/.orchestrator.lock": "session_id: abc",
	} {
		data, err := os.ReadFile(filepath.Join(backupDir, filepath.FromSlash(rel)))
		if err != nil || !strings.HasPrefix(string(data), want) {
			t.Errorf("backup of %s: %v", rel, err)
		}
	}
	userBackup := up.LegacyUserConfig + "." + migrate.BackupSuffix + stamp
	if !slices.Contains(backups, userBackup) {
		t.Errorf("backups %q lack the user file's %s", backups, userBackup)
	}
}

// A repository that already has .ai-workflow.yml with other content is
// refused, and nothing is written.
func TestConflict_BothFilesAreRefused(t *testing.T) {
	w := newWorld(t, comprobantes)
	w.write(".ai-workflow.yml", "enabled: true\n")
	before := treeHash(t, w.root)
	_, err := migrate.Compute(w.options(false, githubOrigin))
	if !refusal.Is(err) {
		t.Fatalf("Compute = %v, want a refusal", err)
	}
	if treeHash(t, w.root) != before {
		t.Error("a refused migration changed files")
	}
	if !strings.Contains(err.Error(), "both exist") {
		t.Errorf("error = %v", err)
	}
}
