package releaseconfig_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
)

// project is a release: section with one project api at the root; extra
// is appended at the project level.
func project(extra string) string {
	return "release:\n  projects:\n    - name: api\n      path: .\n" + extra
}

func TestLoad_ScriptMode(t *testing.T) {
	rel := mustLoad(t, project(`      submodules: true
      build:
        mode: script
        shared_host: true
        version_files: ['src\Api\Api.csproj']
        coverage: { dir: ./tests/coverage/, line: 80, branch: 70.5 }
      stages:
        restore: { run: dotnet restore }
        build: { run: dotnet build -c Release }
        test: { run: dotnet test, timeout: 10m }
        package: { run: dotnet pack -o "$NERV_RELEASE_OUTPUTS_DIR" }
`))
	p := rel.Projects[0]
	b := p.Build
	if b == nil || b.Mode != releaseconfig.ModeScript || !b.SharedHost || !p.Submodules {
		t.Fatalf("build = %+v, submodules %v", b, p.Submodules)
	}
	if !reflect.DeepEqual(b.VersionFiles, []string{"src/Api/Api.csproj"}) {
		t.Errorf("version_files = %v, want the normalised path", b.VersionFiles)
	}
	if b.Coverage == nil || b.Coverage.Dir != "tests/coverage" || b.Coverage.Floors() != (releaseconfig.CoverageThreshold{Line: 80, Branch: 70.5}) {
		t.Errorf("coverage = %+v", b.Coverage)
	}
	if got := p.Stages["package"]; got.Shell != releaseconfig.DefaultShell {
		t.Errorf("stages.package = %+v, want the default shell", got)
	}
	if err := rel.CheckBuild(); err != nil {
		t.Errorf("CheckBuild: %v", err)
	}
}

func TestLoad_ComposeMode(t *testing.T) {
	rel := mustLoad(t, project(`      build:
        mode: compose
        file: deploy\docker-compose.yml
        test_service: tests
        images: ["ghcr.io/acme/api:${version}", "ghcr.io/acme/worker:${version}"]
      stages:
        lint: { run: hadolint Dockerfile }
`))
	b := rel.Projects[0].Build
	if b.Mode != releaseconfig.ModeCompose || b.File != "deploy/docker-compose.yml" || b.TestService != "tests" ||
		!reflect.DeepEqual(b.Images, []releaseconfig.BuildImage{{Ref: "ghcr.io/acme/api:${version}"}, {Ref: "ghcr.io/acme/worker:${version}"}}) {
		t.Errorf("build = %+v", b)
	}
}

// build.mode has no default, and nerv release requires it; loading a section
// without build (nerv version next) still works.
func TestCheckBuild_ModeIsRequiredByRelease(t *testing.T) {
	rel := mustLoad(t, "release:\n  projects:\n    - { name: api, path: api }\n    - name: web\n      path: web\n      build: { mode: script }\n")
	err := rel.CheckBuild()
	var invalid *releaseconfig.ValidationError
	if !errors.As(err, &invalid) || len(invalid.Problems) != 1 ||
		invalid.Problems[0] != "release.projects[0].build.mode: required by nerv release (compose or script): there is no default" {
		t.Fatalf("CheckBuild = %v", err)
	}
}

// Each removed key says exactly what to write instead.
func TestLoad_RemovedPresetKeysSayWhatToWriteInstead(t *testing.T) {
	tests := []struct {
		name, yaml string
		want       []string
	}{
		{"presets", "      presets: [dotnet, docker]\n", []string{
			"release.projects[0].presets: the presets were removed; set build.mode instead:",
			`build: { mode: script }`, `stages: { build: { run: "dotnet build -c Release" } }`,
			`build: { mode: compose, file: docker-compose.yml, test_service: tests, images: ["ghcr.io/org/api:${version}"] }`,
		}},
		{"dotnet", "      dotnet: { project_file: Api.csproj }\n", []string{
			"release.projects[0].dotnet: the dotnet preset was removed; with build: { mode: script } write its commands as stages, e.g.",
			`stages: { restore: { run: "dotnet restore" }, lint: { run: "dotnet format --verify-no-changes" }, build: { run: "dotnet build -c Release --no-restore" }, test: { run: "dotnet test -c Release --no-build" } }`,
			"project_file goes to build.version_files", "coverage_threshold to build.coverage", "shared_host to build.shared_host",
			"sdk: write global.json in a before_restore hook",
		}},
		{"node", "      node: { package_manager: pnpm }\n", []string{
			"release.projects[0].node: the node preset was removed; with build: { mode: script } write its commands as stages, e.g.",
			`stages: { restore: { run: "npm ci" }, lint: { run: "npm run lint" }, build: { run: "npm run build" }, test: { run: "npm test" } }`,
		}},
		{"docker", "      docker: { images: [{ name: ghcr.io/org/api }] }\n", []string{
			"release.projects[0].docker: the docker preset was removed; use",
			`build: { mode: compose, file: docker-compose.yml, images: ["ghcr.io/org/api:${version}"] }`,
			`stages: { package: { run: "docker build -t ghcr.io/org/api:$NERV_RELEASE_VERSION . && docker save -o \"$NERV_RELEASE_OUTPUTS_DIR/api.tar\" ghcr.io/org/api:$NERV_RELEASE_VERSION" } }`,
			"pushing moved to nerv deploy",
		}},
		{"nuget", "      nuget: { source: https://nuget.example/v3/index.json }\n", []string{
			"release.projects[0].nuget: the nuget preset was removed; package in a script stage instead:",
			`stages: { package: { run: "dotnet pack -c Release --no-build -o \"$NERV_RELEASE_OUTPUTS_DIR\"" } }`,
			"pushing moved to nerv deploy",
		}},
		{"npm", "      npm: { access: public }\n", []string{
			"release.projects[0].npm: the npm preset was removed; package in a script stage instead:",
			`stages: { package: { run: "npm pack --pack-destination \"$NERV_RELEASE_OUTPUTS_DIR\"" } }`,
			"publishing moved to nerv deploy",
		}},
		{"publish stage", "      build: { mode: script }\n      stages:\n        publish: { run: x }\n", []string{
			"release.projects[0].stages.publish: the publish stage was removed from nerv release (publishing moved to nerv deploy); write the outputs to $NERV_RELEASE_OUTPUTS_DIR in the package stage instead",
		}},
		{"publish and success hooks", "      build: { mode: script }\n      hooks:\n        publish: [{ run: x }]\n        success: [{ run: y }]\n", []string{
			"release.projects[0].hooks.publish: the publish hooks were removed from nerv release: they belong to nerv deploy now",
			"release.projects[0].hooks.success: the success hooks were removed from nerv release: they belong to nerv deploy now",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := releaseconfig.Load(writeConfig(t, project(tt.yaml)))
			if err == nil {
				t.Fatal("Load succeeded")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error lacks %q:\n%v", w, err)
				}
			}
			if strings.Contains(err.Error(), "unknown field") {
				t.Errorf("a removed key is also reported as an unknown field:\n%v", err)
			}
		})
	}
}

func TestLoad_BuildErrors(t *testing.T) {
	tests := []struct {
		name, yaml, want string
	}{
		{"mode missing in a build block", "      build: { shared_host: true }\n", "release.projects[0].build.mode: required (compose or script)"},
		{"unknown mode", "      build: { mode: docker }\n", `release.projects[0].build.mode: unknown mode "docker" (want compose or script)`},
		{"compose without file", "      build: { mode: compose }\n", "release.projects[0].build.file: required by compose mode"},
		{"compose escaping file", "      build: { mode: compose, file: ../compose.yml }\n", `release.projects[0].build.file: "../compose.yml" escapes the repository root`},
		{"compose bad test service", "      build: { mode: compose, file: c.yml, test_service: \"my tests\" }\n", `release.projects[0].build.test_service: "my tests" is not a compose service name`},
		{"compose empty image", "      build: { mode: compose, file: c.yml, images: [\"\"] }\n", "release.projects[0].build.images[0]: required"},
		{"compose image placeholder", "      build: { mode: compose, file: c.yml, images: [\"x:${build}\"] }\n", "release.projects[0].build.images[0]: unknown placeholder ${build} (want ${version}, ${channel} or ${tag})"},
		{"compose image with a space", "      build: { mode: compose, file: c.yml, images: [\"x y\"] }\n", `release.projects[0].build.images[0]: "x y" is not an image reference`},
		{"compose build stage", "      build: { mode: compose, file: c.yml }\n      stages:\n        build: { run: make }\n", "release.projects[0].stages.build: compose mode runs build itself (docker compose build, run and save); remove it, or use build.mode: script"},
		{"compose shared host", "      build: { mode: compose, file: c.yml, shared_host: true }\n", "release.projects[0].build.shared_host: only for script mode"},
		{"script with compose keys", "      build: { mode: script, file: c.yml, test_service: t, images: [x] }\n", "release.projects[0].build.file: only for compose mode"},
		{"script test service", "      build: { mode: script, test_service: t }\n", "release.projects[0].build.test_service: only for compose mode"},
		{"script image placeholder", "      build: { mode: script, images: [\"x:${build}\"] }\n", "release.projects[0].build.images[0]: unknown placeholder ${build} (want ${version}, ${channel} or ${tag})"},
		{"script image with a space", "      build: { mode: script, images: [\"x y\"] }\n", `release.projects[0].build.images[0]: "x y" is not an image reference`},
		{"version file kind", "      build: { mode: script, version_files: [package.json] }\n", `release.projects[0].build.version_files[0]: "package.json" is not an MSBuild project file (.csproj, .fsproj, .vbproj, .props or .targets)`},
		{"escaping version file", "      build: { mode: script, version_files: [../x.csproj] }\n", `release.projects[0].build.version_files[0]: "../x.csproj" escapes the repository root`},
		{"coverage without floors", "      build: { mode: script, coverage: { dir: cov } }\n", "release.projects[0].build.coverage: set at least one floor (line, branch or method)"},
		{"coverage out of range", "      build: { mode: script, coverage: { line: 120 } }\n", "release.projects[0].build.coverage.line: 120 must be between 0 and 100"},
		{"coverage not a number", "      build: { mode: script, coverage: { method: .nan } }\n", "release.projects[0].build.coverage.method: NaN must be between 0 and 100"},
		{"escaping coverage dir", "      build: { mode: script, coverage: { dir: /tmp, line: 80 } }\n", `release.projects[0].build.coverage.dir: "/tmp" must be relative to the repository root`},
		{"unknown build key", "      build: { mode: script, preset: dotnet }\n", "release.projects[0].build.preset: unknown field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := releaseconfig.Load(writeConfig(t, project(tt.yaml)))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// L22: build.images is accepted in script mode too.
func TestLoad_ScriptModeAcceptsImages(t *testing.T) {
	rel, err := releaseconfig.Load(writeConfig(t, project("      build: { mode: script, images: [\"ghcr.io/acme/api:${version}\", \"ghcr.io/acme/api:${channel}\"] }\n")))
	if err != nil {
		t.Fatal(err)
	}
	if got := rel.Projects[0].Build.Images; len(got) != 2 || !reflect.DeepEqual(got[0], releaseconfig.BuildImage{Ref: "ghcr.io/acme/api:${version}"}) {
		t.Errorf("images = %+v", got)
	}
}

// A build.images entry is a plain reference or a mapping {name, tags}; a
// mapping without tags gets 4.2.0's default tag kinds.
func TestLoad_ImageMappingFormAndPlainString(t *testing.T) {
	rel := mustLoad(t, project(`      build:
        mode: script
        images:
          - "ghcr.io/acme/api:${version}"
          - { name: "ghcr.io/acme/worker" }
          - { name: "registry.local:5000/acme/web", tags: [version, latest] }
`))
	want := []releaseconfig.BuildImage{
		{Ref: "ghcr.io/acme/api:${version}"},
		{Name: "ghcr.io/acme/worker", Tags: []string{"version", "channel", "latest"}},
		{Name: "registry.local:5000/acme/web", Tags: []string{"version", "latest"}},
	}
	if got := rel.Projects[0].Build.Images; !reflect.DeepEqual(got, want) {
		t.Errorf("images = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(releaseconfig.DefaultDockerTags(), []string{"version", "channel", "latest"}) {
		t.Errorf("DefaultDockerTags = %v", releaseconfig.DefaultDockerTags())
	}
}

// The mapping form is checked: known tag kinds only, a repository without
// a tag, and no unknown key.
func TestLoad_ImageMappingIsValidated(t *testing.T) {
	tests := []struct{ name, images, want string }{
		{"unknown tag kind", `[{ name: ghcr.io/acme/api, tags: [version, nightly] }]`, `release.projects[0].build.images[0].tags[1]: unknown tag kind "nightly" (want version, channel or latest)`},
		{"no name", `[{ tags: [version] }]`, "release.projects[0].build.images[0].name: required"},
		{"name with a tag", `[{ name: "ghcr.io/acme/api:1.0" }]`, `release.projects[0].build.images[0].name: "ghcr.io/acme/api:1.0" is not a repository`},
		{"unknown key", `[{ name: ghcr.io/acme/api, push: false }]`, "release.projects[0].build.images[0].push: unknown field"},
		{"a sequence", `[[ghcr.io/acme/api]]`, "a build.images entry is an image reference or a mapping {name, tags}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := releaseconfig.Load(writeConfig(t, project("      build: { mode: script, images: "+tt.images+" }\n")))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
