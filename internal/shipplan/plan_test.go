package shipplan

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// artifacts is the artifacts directory the tests plan with.
var artifacts = filepath.Join(string(filepath.Separator)+"tmp", "nerv-ship")

func ctxFor(version, channel string) Context {
	return Context{Version: version, Channel: channel, Tag: "v" + version, GitHead: "abc123", ArtifactsDir: artifacts}
}

// line renders a step for comparisons: "<stage>|<name>|<command>".
func line(s Step) string {
	cmd := ""
	switch {
	case s.Func != nil:
		cmd = s.Describe
	case len(s.Exec) > 0:
		cmd = strings.Join(s.Exec, " ")
	default:
		cmd = s.Command.Shell + ": " + s.Command.Run
	}
	return s.Stage + "|" + s.Name + "|" + cmd
}

func lines(p *Plan) []string {
	var out []string
	for _, st := range p.Stages {
		for _, s := range st.Steps {
			out = append(out, line(s))
		}
	}
	return out
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("steps:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func project(name, path string, presets ...string) shipconfig.Project {
	return shipconfig.Project{Name: name, Path: path, Presets: presets}
}

func push(b bool) *bool { return &b }

// dimProject is shaped like dim.comprobantes.api's CI: dotnet with coverage
// and two docker images (api and login-broker).
func dimProject() shipconfig.Project {
	p := project("api", ".", "dotnet", "docker")
	p.Dotnet = &shipconfig.Dotnet{ProjectFile: "src/Api/Dim.Comprobantes.Api.csproj", Solution: "Dim.Comprobantes.slnx", Configuration: "Release", CoverageThreshold: 80}
	p.Docker = &shipconfig.Docker{Images: []shipconfig.DockerImage{
		{Name: "ghcr.io/org/dim.comprobantes.api", Dockerfile: "docker/Dockerfile", Context: ".", BuildArgs: map[string]string{"APP_VERSION": "${version}"}, Tags: []string{"version", "channel", "latest"}, Push: push(true)},
		{Name: "ghcr.io/org/dim.comprobantes.api-login-broker", Dockerfile: "docker/LoginBroker.Dockerfile", Context: ".", BuildArgs: map[string]string{"APP_VERSION": "${version}", "RELEASE": "${tag}@${channel}"}, Tags: []string{"version", "channel", "latest"}, Push: push(true)},
	}}
	return p
}

func TestBuild_DotnetAndDockerLikeDim(t *testing.T) {
	plan, err := Build(t.TempDir(), dimProject(), ctxFor("2.9.0-alpha.3", "alpha"))
	if err != nil {
		t.Fatal(err)
	}
	cov := filepath.Join(artifacts, "api", "coverage")
	api, broker := "ghcr.io/org/dim.comprobantes.api", "ghcr.io/org/dim.comprobantes.api-login-broker"
	assertLines(t, lines(plan), []string{
		"restore|dotnet:restore|dotnet restore Dim.Comprobantes.slnx",
		"lint|dotnet:lint|dotnet format Dim.Comprobantes.slnx --verify-no-changes",
		"build|dotnet:build|dotnet build Dim.Comprobantes.slnx -c Release --no-restore -p:Version=2.9.0-alpha.3",
		"test|dotnet:test|dotnet test Dim.Comprobantes.slnx -c Release --no-build --collect XPlat Code Coverage --results-directory " + cov,
		"coverage|dotnet:coverage|check line coverage >= 80% in " + cov,
		"package|docker:build " + api + "|docker build -f docker/Dockerfile -t " + api + ":2.9.0-alpha.3 -t " + api + ":alpha --build-arg APP_VERSION=2.9.0-alpha.3 --label org.opencontainers.image.version=2.9.0-alpha.3 --label org.opencontainers.image.revision=abc123 .",
		"package|docker:build " + broker + "|docker build -f docker/LoginBroker.Dockerfile -t " + broker + ":2.9.0-alpha.3 -t " + broker + ":alpha --build-arg APP_VERSION=2.9.0-alpha.3 --build-arg RELEASE=v2.9.0-alpha.3@alpha --label org.opencontainers.image.version=2.9.0-alpha.3 --label org.opencontainers.image.revision=abc123 .",
		"publish|docker:push " + api + ":2.9.0-alpha.3|docker push " + api + ":2.9.0-alpha.3",
		"publish|docker:push " + api + ":alpha|docker push " + api + ":alpha",
		"publish|docker:push " + broker + ":2.9.0-alpha.3|docker push " + broker + ":2.9.0-alpha.3",
		"publish|docker:push " + broker + ":alpha|docker push " + broker + ":alpha",
	})
	if got := plan.Stages[3].Steps[0].Exec[7]; got != "XPlat Code Coverage" {
		t.Errorf("the collector name is one argument, got %q", got)
	}
	var names []string
	for _, st := range plan.Pipeline() {
		names = append(names, st.Name)
	}
	if want := []string{"restore", "lint", "build", "test", "coverage", "package"}; !slices.Equal(names, want) {
		t.Errorf("Pipeline() = %v, want %v", names, want)
	}
	if pub := plan.Publish(); pub == nil || pub.Name != "publish" || len(pub.Steps) != 4 {
		t.Errorf("Publish() = %+v, want the four pushes", pub)
	}
}

func TestBuild_DotnetWithoutCoverageOrNuget(t *testing.T) {
	p := project("lib", "lib", "dotnet")
	p.Dotnet = &shipconfig.Dotnet{ProjectFile: "lib/Lib.csproj", Configuration: "Debug"}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	assertLines(t, lines(plan), []string{
		"restore|dotnet:restore|dotnet restore lib/Lib.csproj",
		"lint|dotnet:lint|dotnet format lib/Lib.csproj --verify-no-changes",
		"build|dotnet:build|dotnet build lib/Lib.csproj -c Debug --no-restore -p:Version=1.0.0",
		"test|dotnet:test|dotnet test lib/Lib.csproj -c Debug --no-build",
	})
	if plan.Publish() != nil {
		t.Error("Publish() should be nil without publishers")
	}
}

func TestBuild_DockerTagRules(t *testing.T) {
	tests := []struct {
		name    string
		channel string
		tags    []string
		push    bool
		want    []string // image tags, in order
	}{
		{"default channel: version and latest", "", []string{"version", "channel", "latest"}, true, []string{"1.4.0", "latest"}},
		{"named default channel: no channel tag", "default", []string{"version", "channel", "latest"}, true, []string{"1.4.0", "latest"}},
		{"prerelease channel: version and channel", "rc", []string{"version", "channel", "latest"}, true, []string{"1.4.0", "rc"}},
		{"only latest on a prerelease: no tag", "alpha", []string{"latest"}, true, nil},
		{"tag order follows the config", "beta", []string{"channel", "version"}, true, []string{"beta", "1.4.0"}},
		{"no push: built, not pushed", "", []string{"version"}, false, []string{"1.4.0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := project("web", "web", "docker")
			p.Docker = &shipconfig.Docker{Images: []shipconfig.DockerImage{{Name: "reg/web", Dockerfile: "web/Dockerfile", Context: "web", Tags: tt.tags, Push: push(tt.push)}}}
			plan, err := Build(t.TempDir(), p, Context{Version: "1.4.0", Channel: tt.channel, ArtifactsDir: artifacts})
			if err != nil {
				t.Fatal(err)
			}
			var built []string
			for i, a := range plan.Stages[0].Steps[0].Exec {
				if a == "-t" {
					built = append(built, strings.TrimPrefix(plan.Stages[0].Steps[0].Exec[i+1], "reg/web:"))
				}
			}
			if !slices.Equal(built, tt.want) {
				t.Errorf("tags = %v, want %v", built, tt.want)
			}
			var pushed []string
			if pub := plan.Publish(); pub != nil {
				for _, s := range pub.Steps {
					pushed = append(pushed, strings.TrimPrefix(s.Exec[2], "reg/web:"))
				}
			}
			wantPushed := tt.want
			if !tt.push {
				wantPushed = nil
			}
			if !slices.Equal(pushed, wantPushed) {
				t.Errorf("pushed = %v, want %v", pushed, wantPushed)
			}
		})
	}
}

func TestBuild_DockerTagMustBeValid(t *testing.T) {
	p := project("web", "web", "docker")
	p.Docker = &shipconfig.Docker{Images: []shipconfig.DockerImage{{Name: "reg/web", Dockerfile: "Dockerfile", Context: ".", Tags: []string{"version", "channel"}, Push: push(true)}}}
	for _, c := range []Context{
		{Version: "1.0.0+build.5", ArtifactsDir: artifacts},
		{Version: "1.0.0-hotfix-x.1", Channel: "hotfix/x", ArtifactsDir: artifacts},
	} {
		if _, err := Build(t.TempDir(), p, c); err == nil || !strings.Contains(err.Error(), "not a valid docker tag") {
			t.Errorf("Build(%+v) = %v, want an invalid docker tag error", c, err)
		}
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_Node(t *testing.T) {
	tests := []struct {
		name  string
		pm    string
		files map[string]string
		want  []string
	}{
		{
			name:  "npm with every script",
			pm:    "npm",
			files: map[string]string{"web/package.json": `{"scripts":{"lint":"eslint .","build":"vite build","test":"vitest run","dev":"vite"}}`},
			want: []string{
				"restore|node:restore|npm ci",
				"lint|node:lint|npm run lint",
				"build|node:build|npm run build",
				"test|node:test|npm run test",
			},
		},
		{
			name:  "pnpm with only a build script",
			pm:    "pnpm",
			files: map[string]string{"web/package.json": `{"scripts":{"build":"tsc"}}`},
			want: []string{
				"restore|node:restore|pnpm install --frozen-lockfile",
				"build|node:build|pnpm run build",
			},
		},
		{
			name:  "yarn classic",
			pm:    "yarn",
			files: map[string]string{"web/package.json": `{"packageManager":"yarn@1.22.22","scripts":{"test":"jest"}}`},
			want: []string{
				"restore|node:restore|yarn install --frozen-lockfile",
				"test|node:test|yarn run test",
			},
		},
		{
			name:  "yarn berry by packageManager",
			pm:    "yarn",
			files: map[string]string{"web/package.json": `{"packageManager":"yarn@4.5.0+sha512.abc"}`},
			want:  []string{"restore|node:restore|yarn install --immutable"},
		},
		{
			name:  "yarn berry by .yarnrc.yml",
			pm:    "yarn",
			files: map[string]string{"web/package.json": `{}`, "web/.yarnrc.yml": "nodeLinker: node-modules\n"},
			want:  []string{"restore|node:restore|yarn install --immutable"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, c := range tt.files {
				writeFile(t, root, rel, c)
			}
			p := project("web", "web", "node")
			p.Node = &shipconfig.Node{PackageManager: tt.pm}
			plan, err := Build(root, p, ctxFor("1.0.0", ""))
			if err != nil {
				t.Fatal(err)
			}
			assertLines(t, lines(plan), tt.want)
			for _, st := range plan.Stages {
				for _, s := range st.Steps {
					if s.Command.Workdir != "web" {
						t.Errorf("%s runs in %q, want the project path", s.Name, s.Command.Workdir)
					}
				}
			}
		})
	}
}

func TestBuild_NodeNeedsAReadablePackageJSON(t *testing.T) {
	root := t.TempDir()
	p := project("web", "web", "node")
	p.Node = &shipconfig.Node{PackageManager: "npm"}
	if _, err := Build(root, p, ctxFor("1.0.0", "")); err == nil || !strings.Contains(err.Error(), filepath.Join("web", "package.json")) {
		t.Errorf("missing package.json: Build = %v", err)
	}
	writeFile(t, root, "web/package.json", `{"scripts": [}`)
	if _, err := Build(root, p, ctxFor("1.0.0", "")); err == nil || !strings.Contains(err.Error(), "package.json") {
		t.Errorf("invalid package.json: Build = %v", err)
	}
}

func nugetProject() shipconfig.Project {
	p := project("sdk", "sdk", "dotnet", "nuget")
	p.Dotnet = &shipconfig.Dotnet{ProjectFile: "sdk/Sdk.csproj", Configuration: "Release"}
	p.NuGet = &shipconfig.NuGet{Source: "https://nuget.example/v3/index.json", APIKeyEnv: "MY_NUGET_KEY"}
	return p
}

func TestBuild_NuGet(t *testing.T) {
	plan, err := Build(t.TempDir(), nugetProject(), ctxFor("3.1.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	pkgs := filepath.Join(artifacts, "sdk", "nuget")
	got := lines(plan)
	assertLines(t, got[len(got)-2:], []string{
		"package|dotnet:pack|dotnet pack sdk/Sdk.csproj -c Release --no-build -p:Version=3.1.0 -o " + pkgs,
		"publish|nuget:push|push " + filepath.Join(pkgs, "*.nupkg") + " (and *.snupkg) to https://nuget.example/v3/index.json with the API key from $MY_NUGET_KEY, skipping packages already there",
	})
}

func TestBuild_NuGetNeedsPackages(t *testing.T) {
	p := project("sdk", "sdk", "nuget")
	p.NuGet = &shipconfig.NuGet{Source: shipconfig.DefaultNuGetSource, APIKeyEnv: "NUGET_API_KEY"}
	if _, err := Build(t.TempDir(), p, ctxFor("1.0.0", "")); err == nil || !strings.Contains(err.Error(), "nuget preset needs the dotnet preset") {
		t.Errorf("Build = %v, want the nuget-without-packages error", err)
	}
	p.Stages = map[string]shipconfig.Command{"package": {Run: "./pack.sh", Shell: "bash"}}
	if _, err := Build(t.TempDir(), p, ctxFor("1.0.0", "")); err != nil {
		t.Errorf("with a package override: Build = %v", err)
	}
}

func TestPlan_NuGetKeyNeverOnACommandLineOrInTheListing(t *testing.T) {
	feed := &fakeFeed{}
	srv := httptest.NewServer(feed)
	defer srv.Close()
	p := nugetProject()
	p.NuGet.Source = srv.URL + "/v3/index.json"
	c := ctxFor("3.1.0", "")
	c.ArtifactsDir = t.TempDir()
	plan, err := Build(t.TempDir(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, c.ArtifactsDir, "sdk/nuget/Sdk.3.1.0.nupkg", "zip")
	r := &fakeRunner{}
	var out strings.Builder
	e := Executor{Steps: r, Output: &out, LookupEnv: envOf(map[string]string{"MY_NUGET_KEY": testKey})}
	if err := e.RunPublish(context.Background(), plan, shiprun.Vars{Project: "sdk"}); err != nil {
		t.Fatal(err)
	}
	if len(r.steps) != 0 {
		t.Errorf("the push started processes: %v", r.names())
	}
	if len(feed.got) != 2 || feed.got[1].key != testKey {
		t.Errorf("the feed did not get the key in its header: %+v", feed.got)
	}
	for _, text := range []string{plan.Render(), out.String()} {
		if strings.Contains(text, testKey) {
			t.Errorf("the key leaked into %q", text)
		}
	}
	if !strings.Contains(plan.Render(), "$MY_NUGET_KEY") {
		t.Errorf("Render does not name the variable:\n%s", plan.Render())
	}
	for _, st := range plan.Stages {
		for _, s := range st.Steps {
			if slices.Contains(s.Exec, testKey) || strings.Contains(s.Name, testKey) {
				t.Errorf("step %s holds the key", s.Name)
			}
		}
	}

	feed.status, feed.body = http.StatusForbidden, "bad key "+testKey
	err = e.RunPublish(context.Background(), plan, shiprun.Vars{Project: "sdk"})
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Errorf("RunPublish error = %v: want a failure without the key", err)
	}
}

func TestExecutor_MissingSecretStopsBeforeAnything(t *testing.T) {
	plan, err := Build(t.TempDir(), nugetProject(), ctxFor("3.1.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range []map[string]string{{}, {"MY_NUGET_KEY": ""}} {
		r := &fakeRunner{}
		e := Executor{Steps: r, LookupEnv: envOf(env)}
		err := e.Run(context.Background(), plan, shiprun.Vars{Project: "sdk"})
		if err == nil || !strings.Contains(err.Error(), "MY_NUGET_KEY is not set") || !strings.Contains(err.Error(), "nuget:push") {
			t.Errorf("Run = %v, want the missing key named", err)
		}
		if len(r.steps) != 0 {
			t.Errorf("steps ran before the key check: %v", r.names())
		}
	}
	// A dry run never pushes, so it does not need the key.
	dry := ctxFor("3.1.0", "")
	dry.DryRun = true
	plan, _ = Build(t.TempDir(), nugetProject(), dry)
	r := &fakeRunner{}
	if err := (Executor{Steps: r, LookupEnv: envOf(nil)}).Run(context.Background(), plan, shiprun.Vars{}); err != nil {
		t.Errorf("dry run Run = %v", err)
	}
}

func TestBuild_Npm(t *testing.T) {
	tests := []struct {
		name    string
		npm     shipconfig.Npm
		channel string
		want    string
	}{
		{"default channel", shipconfig.Npm{Access: "public"}, "", "npm publish --access public"},
		{"prerelease channel is the dist-tag", shipconfig.Npm{Access: "public"}, "next", "npm publish --access public --tag next"},
		{"dist_tag and registry", shipconfig.Npm{Access: "restricted", DistTag: "beta", Registry: "https://npm.example/"}, "alpha", "npm publish --access restricted --tag beta --registry https://npm.example/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := project("cli", "packages/cli", "npm")
			n := tt.npm
			p.Npm = &n
			plan, err := Build(t.TempDir(), p, ctxFor("2.0.0", tt.channel))
			if err != nil {
				t.Fatal(err)
			}
			assertLines(t, lines(plan), []string{"publish|npm:publish|" + tt.want})
			if wd := plan.Stages[0].Steps[0].Command.Workdir; wd != "packages/cli" {
				t.Errorf("workdir = %q", wd)
			}
		})
	}
}

func TestBuild_StageOverrideReplacesEveryPresetCommand(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"scripts":{"build":"vite build"}}`)
	p := project("app", ".", "dotnet", "node")
	p.Dotnet = &shipconfig.Dotnet{ProjectFile: "App.csproj", Configuration: "Release"}
	p.Node = &shipconfig.Node{PackageManager: "npm"}
	p.Stages = map[string]shipconfig.Command{"build": {Run: "make all", Shell: "bash", Workdir: "src", Timeout: "10m"}}
	plan, err := Build(root, p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	var build *Stage
	for i := range plan.Stages {
		if plan.Stages[i].Name == "build" {
			build = &plan.Stages[i]
		}
	}
	if build == nil {
		t.Fatal("no build stage")
	}
	assertLines(t, []string{line(build.Steps[0])}, []string{"build|build|bash: make all"})
	if len(build.Steps) != 1 || build.Steps[0].Command.Workdir != "src" || build.Steps[0].Command.Timeout != "10m" {
		t.Errorf("build stage = %+v, want only the override, as configured", build.Steps)
	}
	// The other stages keep both presets' commands, in preset order.
	assertLines(t, lines(plan)[:2], []string{
		"restore|dotnet:restore|dotnet restore App.csproj",
		"restore|node:restore|npm ci",
	})
}

func TestBuild_HooksWrapStagesAndEmptyStagesAreSkipped(t *testing.T) {
	p := project("api", ".", "dotnet")
	p.Dotnet = &shipconfig.Dotnet{ProjectFile: "Api.csproj", Configuration: "Release"}
	p.Stages = map[string]shipconfig.Command{"publish": {Run: "./deploy.sh", Shell: "bash"}}
	p.Hooks = map[string][]shipconfig.Command{
		"before_test":     {{Run: "echo one", Shell: "bash"}, {Run: "echo two", Shell: "pwsh"}},
		"after_test":      {{Run: "node report.mjs", Shell: "node"}},
		"before_package":  {{Run: "echo never", Shell: "bash"}}, // no package commands: skipped
		"after_coverage":  {{Run: "echo never", Shell: "bash"}},
		"before_publish":  {{Run: "echo login", Shell: "bash"}},
		"prepare":         {{Run: "./bump.sh", Shell: "bash"}},
		"success":         {{Run: "echo ok", Shell: "bash"}},
		"before_restore":  nil,
		"after_restore":   {},
		"publish":         {{Run: "echo lifecycle", Shell: "bash"}},
		"fail":            {{Run: "echo fail", Shell: "bash"}},
		"before_lint":     {{Run: "echo lint", Shell: "bash"}},
		"after_build":     {{Run: "echo built", Shell: "bash"}},
		"before_coverage": {{Run: "echo never", Shell: "bash"}},
	}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	assertLines(t, lines(plan), []string{
		"restore|dotnet:restore|dotnet restore Api.csproj",
		"before_lint|before_lint|bash: echo lint",
		"lint|dotnet:lint|dotnet format Api.csproj --verify-no-changes",
		"build|dotnet:build|dotnet build Api.csproj -c Release --no-restore -p:Version=1.0.0",
		"after_build|after_build|bash: echo built",
		"before_test|before_test|bash: echo one",
		"before_test|before_test|pwsh: echo two",
		"test|dotnet:test|dotnet test Api.csproj -c Release --no-build",
		"after_test|after_test|node: node report.mjs",
		"before_publish|before_publish|bash: echo login",
		"publish|publish|bash: ./deploy.sh",
	})
	for _, key := range []string{"prepare", "publish", "success", "fail"} {
		if len(plan.Hooks[key]) != 1 || plan.Hooks[key][0].Stage != key {
			t.Errorf("lifecycle hook %s = %+v", key, plan.Hooks[key])
		}
	}
}

func TestBuild_ArtifactsDirIsGivenToEveryStep(t *testing.T) {
	p := dimProject()
	p.Stages = map[string]shipconfig.Command{"lint": {Run: "true", Shell: "bash", Env: map[string]string{"A": "1"}}}
	p.Hooks = map[string][]shipconfig.Command{"after_test": {{Run: "true", Shell: "bash"}}, "prepare": {{Run: "true", Shell: "bash"}}}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(artifacts, "api")
	all := plan.Hooks["prepare"]
	for _, st := range plan.Stages {
		all = append(all, st.Steps...)
	}
	for _, s := range all {
		if s.Func != nil {
			continue
		}
		if got := s.Command.Env[ArtifactsEnv]; got != want {
			t.Errorf("%s: %s = %q, want %q", s.Name, ArtifactsEnv, got, want)
		}
	}
	if p.Stages["lint"].Env[ArtifactsEnv] != "" {
		t.Error("Build changed the configuration's env map")
	}
}

func TestBuild_ContextIsChecked(t *testing.T) {
	p := project("x", ".", "npm")
	p.Npm = &shipconfig.Npm{Access: "public"}
	for _, c := range []Context{
		{ArtifactsDir: artifacts},
		{Version: "1.0.0"},
		{Version: "1.0.0", ArtifactsDir: "relative/dir"},
	} {
		if _, err := Build(t.TempDir(), p, c); err == nil {
			t.Errorf("Build(%+v) = nil error", c)
		}
	}
}

// fakeRunner records the steps it runs and fails those named in fail.
type fakeRunner struct {
	steps []shiprun.Step
	fail  map[string]error
}

func (f *fakeRunner) Run(_ context.Context, s shiprun.Step, _ shiprun.Vars) error {
	f.steps = append(f.steps, s)
	return f.fail[s.Name]
}

func (f *fakeRunner) names() []string {
	var n []string
	for _, s := range f.steps {
		n = append(n, s.Name)
	}
	return n
}

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestExecutor_RunsStagesInOrderAndStopsAtTheFirstFailure(t *testing.T) {
	p := dimProject()
	p.Dotnet.CoverageThreshold = 0
	p.Hooks = map[string][]shipconfig.Command{"after_test": {{Run: "echo after", Shell: "bash"}}}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{}
	if err := (Executor{Steps: r}).Run(context.Background(), plan, shiprun.Vars{Project: "api"}); err != nil {
		t.Fatal(err)
	}
	api, broker := "ghcr.io/org/dim.comprobantes.api", "ghcr.io/org/dim.comprobantes.api-login-broker"
	if want := []string{"dotnet:restore", "dotnet:lint", "dotnet:build", "dotnet:test", "after_test", "docker:build " + api, "docker:build " + broker}; !slices.Equal(r.names(), want) {
		t.Errorf("ran %v, want %v (publish is not part of Run)", r.names(), want)
	}
	if r.steps[0].Stage != "restore" || r.steps[4].Stage != "after_test" || r.steps[0].Exec[0] != "dotnet" {
		t.Errorf("shiprun steps lack their stage or argv: %+v", r.steps[:1])
	}

	stepErr := errors.New("exit code 1")
	r = &fakeRunner{fail: map[string]error{"dotnet:build": stepErr}}
	err = (Executor{Steps: r}).Run(context.Background(), plan, shiprun.Vars{Project: "api"})
	var f *StepFailure
	if !errors.As(err, &f) || f.Stage != "build" || f.Step != "dotnet:build" || !errors.Is(err, stepErr) {
		t.Fatalf("Run = %v, want a StepFailure at build/dotnet:build", err)
	}
	if want := []string{"dotnet:restore", "dotnet:lint", "dotnet:build"}; !slices.Equal(r.names(), want) {
		t.Errorf("ran %v after the failure, want %v", r.names(), want)
	}
}

func TestExecutor_CoverageCheckRunsInProcess(t *testing.T) {
	dir := t.TempDir()
	p := project("api", ".", "dotnet")
	p.Dotnet = &shipconfig.Dotnet{ProjectFile: "Api.csproj", Configuration: "Release", CoverageThreshold: 90}
	plan, err := Build(t.TempDir(), p, Context{Version: "1.0.0", ArtifactsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "api/coverage/0a/coverage.cobertura.xml", cobertura(85, 100))
	var out strings.Builder
	r := &fakeRunner{}
	err = (Executor{Steps: r, Output: &out}).Run(context.Background(), plan, shiprun.Vars{Project: "api"})
	var f *StepFailure
	if !errors.As(err, &f) || f.Stage != "coverage" || f.Step != "dotnet:coverage" || !strings.Contains(err.Error(), "85.00%") {
		t.Fatalf("Run = %v, want the coverage check failure", err)
	}
	if slices.Contains(r.names(), "dotnet:coverage") {
		t.Error("the check was sent to the process runner")
	}

	writeFile(t, dir, "api/coverage/0a/coverage.cobertura.xml", cobertura(95, 100))
	out.Reset()
	if err := (Executor{Steps: &fakeRunner{}, Output: &out}).Run(context.Background(), plan, shiprun.Vars{Project: "api"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[api:dotnet:coverage] line coverage 95.00% (95 of 100 lines, 1 report), threshold 90%") {
		t.Errorf("output = %q", out.String())
	}
}

func TestExecutor_PublishIsNotRunInADryRun(t *testing.T) {
	c := ctxFor("1.0.0", "")
	c.DryRun = true
	plan, err := Build(t.TempDir(), dimProject(), c)
	if err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{}
	if err := (Executor{Steps: r}).RunPublish(context.Background(), plan, shiprun.Vars{}); err != nil {
		t.Fatal(err)
	}
	if len(r.steps) != 0 {
		t.Errorf("dry run published: %v", r.names())
	}
	if !strings.Contains(plan.Render(), "publish (not run: dry run)") {
		t.Errorf("Render does not mark the skipped publish:\n%s", plan.Render())
	}
}

func TestPlan_Render(t *testing.T) {
	p := dimProject()
	p.Docker.Images = p.Docker.Images[:1]
	p.Hooks = map[string][]shipconfig.Command{
		"after_test": {{Run: "scripts/report.ps1", Shell: "pwsh", Workdir: "tools"}},
		"prepare":    {{Run: "echo a\necho b", Shell: "bash"}},
	}
	plan, err := Build(t.TempDir(), p, ctxFor("1.0.0", ""))
	if err != nil {
		t.Fatal(err)
	}
	cov := filepath.Join(artifacts, "api", "coverage")
	want := `api 1.0.0 (default channel, tag v1.0.0)
  restore
    dotnet:restore  dotnet restore Dim.Comprobantes.slnx
  lint
    dotnet:lint  dotnet format Dim.Comprobantes.slnx --verify-no-changes
  build
    dotnet:build  dotnet build Dim.Comprobantes.slnx -c Release --no-restore -p:Version=1.0.0
  test
    dotnet:test  dotnet test Dim.Comprobantes.slnx -c Release --no-build --collect "XPlat Code Coverage" --results-directory ` + cov + `
    after_test  [pwsh in tools] scripts/report.ps1
  coverage
    dotnet:coverage  check line coverage >= 80% in ` + cov + `
  package
    docker:build ghcr.io/org/dim.comprobantes.api  docker build -f docker/Dockerfile -t ghcr.io/org/dim.comprobantes.api:1.0.0 -t ghcr.io/org/dim.comprobantes.api:latest --build-arg APP_VERSION=1.0.0 --label org.opencontainers.image.version=1.0.0 --label org.opencontainers.image.revision=abc123 .
  publish (runs at semantic-release's publish step)
    docker:push ghcr.io/org/dim.comprobantes.api:1.0.0  docker push ghcr.io/org/dim.comprobantes.api:1.0.0
    docker:push ghcr.io/org/dim.comprobantes.api:latest  docker push ghcr.io/org/dim.comprobantes.api:latest
  hooks at semantic-release steps
    prepare  [bash] echo a
        echo b
`
	if got := plan.Render(); got != want {
		t.Errorf("Render =\n%s\nwant\n%s", got, want)
	}
}
