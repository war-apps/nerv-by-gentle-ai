package pipelineconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
)

// writeConfig writes text as <repo>/.nerv/nerv.yaml in a fresh temporary
// repository and returns the repository root.
func writeConfig(t *testing.T, text string) string {
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

func mustLoad(t *testing.T, text string) *pipelineconfig.Pipeline {
	t.Helper()
	p, err := pipelineconfig.Load(writeConfig(t, text))
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	return p
}

// minimal is the smallest valid section: a solution and one image.
const minimal = `pipeline:
  dotnet:
    solution: App.slnx
  images:
    - name: ghcr.io/acme/app
`

func TestLoad_NotConfigured(t *testing.T) {
	tests := []struct {
		name string
		repo func(t *testing.T) string
	}{
		{"no nerv.yaml", func(t *testing.T) string { return t.TempDir() }},
		{"nerv.yaml without pipeline", func(t *testing.T) string {
			return writeConfig(t, "enabled: true\nrelease:\n  projects:\n    - { name: api, path: . }\n")
		}},
		{"empty nerv.yaml", func(t *testing.T) string { return writeConfig(t, "") }},
		{"broken YAML elsewhere", func(t *testing.T) string {
			return writeConfig(t, "enabled: true\nenabled: false\n")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := pipelineconfig.Load(tt.repo(t))
			if !errors.Is(err, pipelineconfig.ErrNotConfigured) {
				t.Fatalf("err = %v, want ErrNotConfigured", err)
			}
			if p != nil {
				t.Fatalf("pipeline = %+v, want nil", p)
			}
		})
	}
}

// The section is read on its own: YAML errors outside it and a release:
// section next to it do not matter.
func TestLoad_IndependentOfTheRestOfTheFile(t *testing.T) {
	tests := []struct{ name, yaml string }{
		{"duplicate key before", "enabled: true\nenabled: false\n" + minimal},
		{"tab indentation after", minimal + "models:\n\tmisato: fable\n"},
		{"release section before", "release:\n  projects: [oops\n" + minimal},
		{"crlf line endings", strings.ReplaceAll("enabled: true\n"+minimal, "\n", "\r\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustLoad(t, tt.yaml)
			if p.Dotnet.Solution != "App.slnx" || len(p.Images) != 1 {
				t.Fatalf("pipeline = %+v", p)
			}
		})
	}
}

func TestLoad_Defaults(t *testing.T) {
	p := mustLoad(t, minimal)

	if p.Dotnet.Configuration != "Release" || !p.Dotnet.TreatWarningsAsErrors() || p.Dotnet.Runsettings != "" || len(p.Dotnet.Env) != 0 {
		t.Errorf("dotnet = %+v", p.Dotnet)
	}
	wantBranches := []pipelineconfig.Branch{
		{Name: "main"},
		{Name: "develop", Prerelease: "alpha", Channel: "alpha"},
		{Name: "release/*", Prerelease: "rc", Channel: "beta"},
		{Name: "hotfix/*", Prerelease: "beta", Channel: "beta"},
	}
	if !reflect.DeepEqual(p.Branches, wantBranches) {
		t.Errorf("branches = %+v, want %+v", p.Branches, wantBranches)
	}
	if p.Registry != (pipelineconfig.Registry{Host: "ghcr.io", UsernameEnv: "GHCR_USER", TokenEnv: "GHCR_TOKEN"}) {
		t.Errorf("registry = %+v", p.Registry)
	}
	img := p.Images[0]
	if img.Dockerfile != "Dockerfile" || img.Context != "." || img.Platform != "linux/amd64" || img.Title != "app" ||
		!img.IsRequired() || !reflect.DeepEqual(img.Tags, []string{"version", "channel", "latest"}) {
		t.Errorf("image = %+v", img)
	}
	if !p.Sync.IsEnabled() || p.Sync.Develop != "develop" || p.Sync.Release != "release/*" {
		t.Errorf("sync = %+v", p.Sync)
	}
	if p.Deploy != nil {
		t.Errorf("deploy = %+v, want nil when not configured", p.Deploy)
	}
	if p.MainBranch() != "main" {
		t.Errorf("MainBranch() = %q", p.MainBranch())
	}
}

func TestLoad_DeployDefaults(t *testing.T) {
	p := mustLoad(t, minimal+"  deploy: {}\n")
	d := p.Deploy
	if d == nil {
		t.Fatal("deploy = nil")
	}
	if d.Source != "env" || d.Infisical != nil {
		t.Errorf("source = %q infisical = %+v", d.Source, d.Infisical)
	}
	if want := []string{"production-green", "production-blue", "staging", "development"}; !reflect.DeepEqual(d.Environments, want) {
		t.Errorf("environments = %v, want %v", d.Environments, want)
	}
	if d.SSH.Auth != "password" || d.SSH.Port != 22 || d.SSH.KnownHosts != "~/.ssh/known_hosts" ||
		d.SSH.KeyFile != "" || d.SSH.ConnectTimeoutDuration() != 10*time.Second {
		t.Errorf("ssh = %+v", d.SSH)
	}
	if !d.Verify.CheckHealthy() || d.Verify.VersionLabel != "org.opencontainers.image.version" || d.Verify.TimeoutDuration() != 120*time.Second {
		t.Errorf("verify = %+v", d.Verify)
	}
}

func TestLoad_ExplicitValues(t *testing.T) {
	p := mustLoad(t, `pipeline:
  dotnet:
    solution: src\App.slnx
    configuration: Debug
    runsettings: ./coverlet.runsettings
    warnings_as_errors: false
    env: { MSBUILDDISABLENODEREUSE: "1", UseSharedCompilation: "false" }
  branches:
    - { name: master }
    - { name: next, prerelease: next }
  registry: { host: registry.example.com:5000, username_env: REG_USER, token_env: REG_TOKEN }
  images:
    - name: registry.example.com:5000/Acme/App
      title: the app
      dockerfile: docker/App.Dockerfile
      context: src/
      platform: linux/arm64
      build_args: { APP_VERSION: "${version}", CHANNEL: "${channel}", TAG: "${tag}" }
      computed_args:
        CHROME_VERSION: { run: "echo 1.2.3" }
        NODE_VERSION: { run: "node -p process.version", shell: node }
      tags: [version]
      required: false
  sync: { enabled: false, develop: next, release: "support/*" }
  deploy:
    source: infisical
    infisical: { project_id: abc-123, app_path: /app, domain: "https://infisical.example.com" }
    environments: [staging]
    ssh: { auth: key, key_file: ~/.ssh/deploy, known_hosts: ~/.config/known_hosts, port: 2222, connect_timeout: 30s }
    verify: { healthy: false, version_label: com.example.version, timeout: 5m }
`)
	if p.Dotnet.Solution != "src/App.slnx" || p.Dotnet.Runsettings != "coverlet.runsettings" || p.Dotnet.Configuration != "Debug" ||
		p.Dotnet.TreatWarningsAsErrors() || p.Dotnet.Env["UseSharedCompilation"] != "false" {
		t.Errorf("dotnet = %+v", p.Dotnet)
	}
	if want := []pipelineconfig.Branch{{Name: "master"}, {Name: "next", Prerelease: "next", Channel: "next"}}; !reflect.DeepEqual(p.Branches, want) {
		t.Errorf("branches = %+v, want %+v (channel defaults to the prerelease id)", p.Branches, want)
	}
	img := p.Images[0]
	if img.Name != "registry.example.com:5000/acme/app" {
		t.Errorf("name = %q, want it lowercased", img.Name)
	}
	if img.Title != "the app" || img.Dockerfile != "docker/App.Dockerfile" || img.Context != "src" || img.Platform != "linux/arm64" || img.IsRequired() {
		t.Errorf("image = %+v", img)
	}
	if img.ComputedArgs["CHROME_VERSION"].Shell != "bash" || img.ComputedArgs["NODE_VERSION"].Shell != "node" {
		t.Errorf("computed_args = %+v (shell defaults to bash)", img.ComputedArgs)
	}
	if p.Sync.IsEnabled() || p.Sync.Develop != "next" || p.Sync.Release != "support/*" {
		t.Errorf("sync = %+v", p.Sync)
	}
	d := p.Deploy
	if d.Source != "infisical" || d.Infisical.ProjectID != "abc-123" || d.Infisical.AppPath != "/app" {
		t.Errorf("deploy = %+v", d)
	}
	if d.SSH.Auth != "key" || d.SSH.KeyFile != "~/.ssh/deploy" || d.SSH.Port != 2222 || d.SSH.ConnectTimeoutDuration() != 30*time.Second {
		t.Errorf("ssh = %+v", d.SSH)
	}
	if d.Verify.CheckHealthy() || d.Verify.VersionLabel != "com.example.version" || d.Verify.TimeoutDuration() != 5*time.Minute {
		t.Errorf("verify = %+v", d.Verify)
	}
}

// Every problem is reported at once, each with its path in the section.
func TestLoad_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{"not a mapping", "pipeline: [a]\n", []string{"pipeline: must be a mapping"}},
		{"empty section", "pipeline:\n", []string{"pipeline.dotnet.solution: required", "pipeline.images: at least one image is required"}},
		{"unknown keys", minimal + "  extra: 1\n  dotnet_x: {}\n", []string{"pipeline.extra: unknown field (line 6)", "pipeline.dotnet_x: unknown field (line 7)"}},
		{"unknown nested key", `pipeline:
  dotnet: { solution: App.slnx, sln: x }
  images:
    - { name: ghcr.io/acme/app, file: Dockerfile }
`, []string{"pipeline.dotnet.sln: unknown field (line 2)", "pipeline.images[0].file: unknown field (line 4)"}},
		{"wrong kind", "pipeline:\n  dotnet: { solution: App.slnx }\n  images: ghcr.io/acme/app\n", []string{"pipeline: line 3: cannot unmarshal"}},
		{"absolute solution", strings.Replace(minimal, "App.slnx", "/abs/App.slnx", 1), []string{`pipeline.dotnet.solution: "/abs/App.slnx" must be relative to the repository root`}},
		{"bad dotnet env name", `pipeline:
  dotnet: { solution: App.slnx, env: { "BAD-NAME": "1" } }
  images: [{ name: ghcr.io/acme/app }]
`, []string{"pipeline.dotnet.env.BAD-NAME: not a valid environment variable name"}},
		{"bad image names", `pipeline:
  dotnet: { solution: App.slnx }
  images:
    - { name: "" }
    - { name: ghcr.io/acme/app:1.0 }
    - { name: "ghcr.io/acme/my app" }
    - { name: docker.io/acme/app }
    - { name: ghcr.io/acme/app }
    - { name: ghcr.io/acme/app }
`, []string{
			"pipeline.images[0].name: required",
			`pipeline.images[1].name: "ghcr.io/acme/app:1.0" is not a valid image repository (lowercase path components, no tag or digest)`,
			`pipeline.images[2].name: "ghcr.io/acme/my app" is not a valid image repository (lowercase path components, no tag or digest)`,
			`pipeline.images[3].name: "docker.io/acme/app" is not on the registry host ghcr.io (pipeline.registry.host)`,
			`pipeline.images[5].name: duplicate image "ghcr.io/acme/app"`,
		}},
		{"image paths, platform and tags", `pipeline:
  dotnet: { solution: App.slnx }
  images:
    - name: ghcr.io/acme/app
      dockerfile: C:\docker\Dockerfile
      context: ../..
      platform: amd64
      tags: [version, nightly]
`, []string{
			`pipeline.images[0].dockerfile: "C:\\docker\\Dockerfile" must be relative to the repository root`,
			`pipeline.images[0].context: "../.." escapes the repository root`,
			`pipeline.images[0].platform: "amd64" is not a platform such as linux/amd64`,
			`pipeline.images[0].tags[1]: unknown tag "nightly" (want version, channel or latest)`,
		}},
		{"build args and computed args", `pipeline:
  dotnet: { solution: App.slnx }
  images:
    - name: ghcr.io/acme/app
      build_args: { APP_VERSION: "${version}-${sha}", "BAD-ARG": x }
      computed_args:
        CHROME: { run: "", shell: fish }
        APP_VERSION: { run: echo 1 }
`, []string{
			"pipeline.images[0].build_args.APP_VERSION: unknown placeholder ${sha} (want ${version}, ${channel} or ${tag})",
			"pipeline.images[0].build_args.BAD-ARG: not a valid build argument name",
			"pipeline.images[0].computed_args.APP_VERSION: also given in build_args",
			"pipeline.images[0].computed_args.CHROME.run: required",
			`pipeline.images[0].computed_args.CHROME.shell: unknown shell "fish" (want node, pwsh or bash)`,
		}},
		{"bad branches", `pipeline:
  dotnet: { solution: App.slnx }
  images: [{ name: ghcr.io/acme/app }]
  branches:
    - { name: develop, prerelease: alpha }
    - { name: "" }
    - { name: develop, prerelease: beta }
    - { name: "release/*", prerelease: "rc.1" }
    - { name: "feat ure", prerelease: x, channel: latest }
    - { name: "hotfix/[", prerelease: true }
`, []string{
			"pipeline.branches[1].name: required",
			`pipeline.branches[2].name: duplicate branch "develop"`,
			`pipeline.branches[3].prerelease: "rc.1" is not a valid prerelease id (a letter, then letters, digits or -)`,
			`pipeline.branches[4].name: "feat ure" is not a valid branch name or pattern`,
			`pipeline.branches[4].channel: "latest" is reserved for the main branch image tag`,
			`pipeline.branches[5].name: "hotfix/[" is not a valid branch name or pattern`,
			"pipeline.branches[5].prerelease: true is not supported; give a fixed id such as beta",
			"pipeline.branches: at least one release branch (without prerelease) is required",
		}},
		{"main branch is a pattern", `pipeline:
  dotnet: { solution: App.slnx }
  images: [{ name: ghcr.io/acme/app }]
  sync: { enabled: false }
  branches: [{ name: "release/*" }, { name: develop, prerelease: alpha }]
`, []string{`pipeline.branches[0].name: "release/*" is the main branch (the first release branch) and must be a branch name, not a pattern`}},
		{"solution with surrounding spaces", strings.Replace(minimal, "App.slnx", `" App.slnx "`, 1), []string{`pipeline.dotnet.solution: " App.slnx " has leading or trailing spaces`}},
		{"too many release branches", `pipeline:
  dotnet: { solution: App.slnx }
  images: [{ name: ghcr.io/acme/app }]
  sync: { enabled: false }
  branches: [{ name: a }, { name: b }, { name: c }, { name: d }]
`, []string{"pipeline.branches: at most 3 release branches (without prerelease) are allowed, got 4"}},
		{"sync develop not a branch", `pipeline:
  dotnet: { solution: App.slnx }
  images: [{ name: ghcr.io/acme/app }]
  branches: [{ name: main }, { name: next, prerelease: next }]
  sync: { release: "" }
`, []string{
			`pipeline.sync.develop: "develop" is not one of pipeline.branches`,
		}},
		{"sync develop wildcard", `pipeline:
  dotnet: { solution: App.slnx }
  images: [{ name: ghcr.io/acme/app }]
  branches: [{ name: main }, { name: "dev/*", prerelease: dev }]
  sync: { develop: "dev/*" }
`, []string{`pipeline.sync.develop: "dev/*" must be a single branch, not a pattern`}},
		{"registry secrets given as values", `pipeline:
  dotnet: { solution: App.slnx }
  images: [{ name: ghcr.io/acme/app }]
  registry: { username_env: my-user, token_env: ghp_abcdefABCDEF0123456789 }
`, []string{
			"pipeline.registry.username_env: must be the name of an environment variable (A-Z, 0-9, _), not its value",
			"pipeline.registry.token_env: must be the name of an environment variable (A-Z, 0-9, _), not its value",
		}},
		{"bad registry host", `pipeline:
  dotnet: { solution: App.slnx }
  images: [{ name: ghcr.io/acme/app }]
  registry: { host: "https://ghcr.io" }
`, []string{
			`pipeline.registry.host: "https://ghcr.io" is not a registry host such as ghcr.io`,
			`pipeline.images[0].name: "ghcr.io/acme/app" is not on the registry host https://ghcr.io (pipeline.registry.host)`,
		}},
		{"unknown deploy source", minimal + "  deploy: { source: vault }\n", []string{`pipeline.deploy.source: unknown source "vault" (want env or infisical)`}},
		{"infisical without settings", minimal + "  deploy: { source: infisical }\n", []string{
			"pipeline.deploy.infisical.project_id: required by source infisical",
			"pipeline.deploy.infisical.app_path: required by source infisical",
		}},
		{"infisical with bad settings", minimal + `  deploy:
    source: infisical
    infisical: { project_id: " ", app_path: app/, domain: "ftp://x" }
`, []string{
			"pipeline.deploy.infisical.project_id: required by source infisical",
			`pipeline.deploy.infisical.app_path: "app/" must be an Infisical folder path such as /my-app`,
			`pipeline.deploy.infisical.domain: "ftp://x" must be an http(s) URL`,
		}},
		{"infisical settings with source env", minimal + "  deploy: { infisical: { project_id: x, app_path: /x } }\n", []string{
			"pipeline.deploy.infisical: settings given but the source is env",
		}},
		{"empty environments", minimal + "  deploy: { environments: [] }\n", []string{"pipeline.deploy.environments: at least one environment is required"}},
		{"bad environments", minimal + "  deploy: { environments: [staging, \"\", staging, prod blue, production-blue, production_blue] }\n", []string{
			"pipeline.deploy.environments[1]: required",
			`pipeline.deploy.environments[2]: duplicate environment "staging"`,
			`pipeline.deploy.environments[3]: "prod blue" must match [A-Za-z0-9][A-Za-z0-9_-]*`,
			`pipeline.deploy.environments[5]: "production_blue" and "production-blue" share the variable prefix NERV_DEPLOY_PRODUCTION_BLUE_`,
		}},
		{"bad ssh and verify", minimal + `  deploy:
    ssh: { auth: token, port: 70000, connect_timeout: soon, known_hosts: "" }
    verify: { version_label: "", timeout: -1s }
`, []string{
			`pipeline.deploy.ssh.auth: unknown auth "token" (want password or key)`,
			"pipeline.deploy.ssh.port: 70000 must be between 1 and 65535",
			`pipeline.deploy.ssh.connect_timeout: "soon" is not a duration such as 10s or 2m`,
			`pipeline.deploy.verify.timeout: "-1s" must be positive`,
		}},
		{"key auth without key file", minimal + "  deploy: { ssh: { auth: key } }\n", []string{"pipeline.deploy.ssh.key_file: required by auth key"}},
		{"key file with password auth", minimal + "  deploy: { ssh: { key_file: ~/.ssh/id } }\n", []string{"pipeline.deploy.ssh.key_file: only used by auth key"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pipelineconfig.Load(writeConfig(t, tt.yaml))
			var verr *pipelineconfig.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("err = %v, want a *ValidationError", err)
			}
			if tt.want == nil {
				return
			}
			for _, want := range tt.want {
				if !containsPrefix(verr.Problems, want) {
					t.Errorf("missing problem %q in:\n  %s", want, strings.Join(verr.Problems, "\n  "))
				}
			}
			if !strings.HasPrefix(tt.name, "wrong kind") && len(verr.Problems) != len(tt.want) {
				t.Errorf("got %d problems, want %d:\n  %s", len(verr.Problems), len(tt.want), strings.Join(verr.Problems, "\n  "))
			}
		})
	}
}

func containsPrefix(problems []string, want string) bool {
	for _, p := range problems {
		if strings.HasPrefix(p, want) {
			return true
		}
	}
	return false
}

// A path that escapes the repository is reported for runsettings too.
func TestLoad_RunsettingsEscapes(t *testing.T) {
	_, err := pipelineconfig.Load(writeConfig(t, `pipeline:
  dotnet: { solution: App.slnx, runsettings: ../x.runsettings }
  images: [{ name: ghcr.io/acme/app }]
`))
	if err == nil || !strings.Contains(err.Error(), `pipeline.dotnet.runsettings: "../x.runsettings" escapes the repository root`) {
		t.Fatalf("err = %v", err)
	}
}

// A secret given where an environment variable name is expected is never
// echoed back in the error.
func TestLoad_SecretValuesAreNotEchoed(t *testing.T) {
	const secret = "ghp_abcdefABCDEF0123456789"
	_, err := pipelineconfig.Load(writeConfig(t, minimal+"  registry: { token_env: "+secret+" }\n"))
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("the error echoes the secret: %v", err)
	}
}

// Anchors and merge keys inside the section work; an unknown key reached
// only through a merge is still reported.
func TestLoad_MergeKeys(t *testing.T) {
	p := mustLoad(t, `pipeline:
  dotnet: { solution: App.slnx }
  images:
    - &base
      name: ghcr.io/acme/app
      build_args: { APP_VERSION: "${version}" }
    - <<: *base
      name: ghcr.io/acme/app-worker
`)
	if len(p.Images) != 2 || p.Images[1].BuildArgs["APP_VERSION"] != "${version}" {
		t.Fatalf("images = %+v", p.Images)
	}
	_, err := pipelineconfig.Load(writeConfig(t, `pipeline:
  dotnet: { solution: App.slnx }
  images:
    - &base
      name: ghcr.io/acme/app
      bogus: 1
    - <<: *base
      name: ghcr.io/acme/app-worker
`))
	if err == nil || !strings.Contains(err.Error(), "pipeline.images[0].bogus: unknown field") ||
		!strings.Contains(err.Error(), "pipeline.images[1].bogus: unknown field") {
		t.Fatalf("err = %v", err)
	}
}

func TestPipeline_Branch(t *testing.T) {
	p := mustLoad(t, minimal)
	tests := []struct {
		branch      string
		want        string
		wantChannel string
		ok          bool
	}{
		{"main", "main", "", true},
		{"develop", "develop", "alpha", true},
		{"release/rc", "release/*", "beta", true},
		{"release/1.4", "release/*", "beta", true},
		{"hotfix/fix-auth", "hotfix/*", "beta", true},
		{"release/a/b", "", "", false},
		{"feature/x", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			b, ok := p.Branch(tt.branch)
			if ok != tt.ok || b.Name != tt.want || b.Channel != tt.wantChannel {
				t.Fatalf("Branch(%q) = %+v, %v; want %q (channel %q), %v", tt.branch, b, ok, tt.want, tt.wantChannel, tt.ok)
			}
		})
	}
}

func TestDeployEnvVar(t *testing.T) {
	tests := []struct{ env, key, want string }{
		{"production-blue", "DOCKER_SSH_HOST", "NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_SSH_HOST"},
		{"staging", "DOCKER_CONTAINER_NAME", "NERV_DEPLOY_STAGING_DOCKER_CONTAINER_NAME"},
		{"Dev_2", "DOCKER_SSH_PASS", "NERV_DEPLOY_DEV_2_DOCKER_SSH_PASS"},
	}
	for _, tt := range tests {
		if got := pipelineconfig.DeployEnvVar(tt.env, tt.key); got != tt.want {
			t.Errorf("DeployEnvVar(%q, %q) = %q, want %q", tt.env, tt.key, got, tt.want)
		}
	}
}

// The returned defaults are fresh copies: a caller cannot change later loads.
func TestDefaults_ReturnCopies(t *testing.T) {
	b := pipelineconfig.DefaultBranches()
	b[0].Name = "mutated"
	e := pipelineconfig.DefaultEnvironments()
	e[0] = "mutated"
	if pipelineconfig.DefaultBranches()[0].Name != "main" || pipelineconfig.DefaultEnvironments()[0] != "production-green" {
		t.Fatal("defaults were mutated through a returned slice")
	}
}

const docPath = "../../docs/pipeline.md"

// yamlBlocks returns every ```yaml block of a markdown text that starts with
// a pipeline: key.
func yamlBlocks(t *testing.T, doc string) []string {
	t.Helper()
	var blocks []string
	rest := doc
	for {
		_, after, ok := strings.Cut(rest, "```yaml\n")
		if !ok {
			break
		}
		block, tail, closed := strings.Cut(after, "```")
		if !closed {
			t.Fatal("unclosed yaml block")
		}
		if strings.HasPrefix(block, "pipeline:") {
			blocks = append(blocks, block)
		}
		rest = tail
	}
	return blocks
}

// Every pipeline: example of docs/pipeline.md must stay loadable, and the
// first one is the complete dim.comprobantes.api example.
func TestLoad_DocumentedExamples(t *testing.T) {
	doc, err := os.ReadFile(filepath.FromSlash(docPath))
	if err != nil {
		t.Fatal(err)
	}
	blocks := yamlBlocks(t, string(doc))
	if len(blocks) < 2 {
		t.Fatalf("docs/pipeline.md has %d pipeline: examples, want at least 2 (env and infisical deploy)", len(blocks))
	}
	sources := map[string]bool{}
	for i, block := range blocks {
		p, err := pipelineconfig.Load(writeConfig(t, block))
		if err != nil {
			t.Fatalf("example %d: %v", i+1, err)
		}
		if p.Deploy != nil {
			sources[p.Deploy.Source] = true
		}
	}
	if !sources["env"] || !sources["infisical"] {
		t.Errorf("deploy sources documented = %v, want env and infisical", sources)
	}
	full := mustLoad(t, blocks[0])
	if len(full.Images) != 2 || full.Dotnet.Solution != "Dim.Comprobantes.slnx" || full.Dotnet.Runsettings != "coverlet.runsettings" {
		t.Errorf("complete example loaded as %+v", full)
	}
}

// The documented defaults are the documented values: a change to a default
// must be a deliberate change to docs/pipeline.md too.
func TestDefaults_MatchDocs(t *testing.T) {
	want := map[string]string{
		pipelineconfig.DefaultDotnetConfiguration: "Release",
		pipelineconfig.DefaultRegistryHost:        "ghcr.io",
		pipelineconfig.DefaultRegistryUsernameEnv: "GHCR_USER",
		pipelineconfig.DefaultRegistryTokenEnv:    "GHCR_TOKEN",
		pipelineconfig.DefaultDockerfile:          "Dockerfile",
		pipelineconfig.DefaultDockerContext:       ".",
		pipelineconfig.DefaultPlatform:            "linux/amd64",
		pipelineconfig.DefaultShell:               "bash",
		pipelineconfig.DefaultSyncDevelop:         "develop",
		pipelineconfig.DefaultSyncRelease:         "release/*",
		pipelineconfig.DefaultDeploySource:        "env",
		pipelineconfig.DefaultSSHAuth:             "password",
		pipelineconfig.DefaultSSHKnownHosts:       "~/.ssh/known_hosts",
		pipelineconfig.DefaultSSHConnectTimeout:   "10s",
		pipelineconfig.DefaultVersionLabel:        "org.opencontainers.image.version",
		pipelineconfig.DefaultVerifyTimeout:       "120s",
		pipelineconfig.DeployEnvPrefix:            "NERV_DEPLOY",
	}
	doc, err := os.ReadFile(filepath.FromSlash(docPath))
	if err != nil {
		t.Fatal(err)
	}
	for got, documented := range want {
		if got != documented {
			t.Errorf("default %q, documented as %q", got, documented)
		}
		if !strings.Contains(string(doc), "`"+documented+"`") {
			t.Errorf("docs/pipeline.md does not mention the default `%s`", documented)
		}
	}
	if pipelineconfig.DefaultSSHPort != 22 || !strings.Contains(string(doc), "`22`") {
		t.Errorf("DefaultSSHPort = %d, documented as `22`", pipelineconfig.DefaultSSHPort)
	}
	for _, env := range pipelineconfig.DefaultEnvironments() {
		if !strings.Contains(string(doc), env) {
			t.Errorf("docs/pipeline.md does not mention the default environment %s", env)
		}
	}
	for _, key := range append(pipelineconfig.DeployHostKeys(), pipelineconfig.DeployAppKeys()...) {
		if !strings.Contains(string(doc), "`"+key+"`") {
			t.Errorf("docs/pipeline.md does not document the deploy key `%s`", key)
		}
	}
}

func TestLoad_BranchPatternsOnlySupportStarQuestionAndClasses(t *testing.T) {
	for _, name := range []string{"release/**", "release/{a,b}", "release/}", "release/@(a|b)", "release/!(x)", "release/+(x)", "release/*(x)", "release/?(x)"} {
		t.Run(name, func(t *testing.T) {
			text := strings.Replace(minimal, "pipeline:\n", "pipeline:\n  branches:\n    - name: main\n    - name: develop\n      prerelease: alpha\n    - name: \""+name+"\"\n      prerelease: rc\n", 1)
			_, err := pipelineconfig.Load(writeConfig(t, text))
			want := `pipeline.branches[2].name: "` + name + `" uses glob syntax nerv does not support (only *, ? and [...] are)`
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("err = %v, want %q", err, want)
			}
		})
	}
	for _, name := range []string{"release/*", "release/v?", "hotfix/[a-z]*"} {
		text := strings.Replace(minimal, "pipeline:\n", "pipeline:\n  branches:\n    - name: main\n    - name: develop\n      prerelease: alpha\n    - name: \""+name+"\"\n      prerelease: rc\n", 1)
		if _, err := pipelineconfig.Load(writeConfig(t, text)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestLoad_ReleaseVersionFiles(t *testing.T) {
	p := mustLoad(t, minimal+"  release:\n    version_files: [src\\Api\\Api.csproj, src/Lib/Lib.csproj]\n")
	if want := []string{"src/Api/Api.csproj", "src/Lib/Lib.csproj"}; !slices.Equal(p.Release.VersionFiles, want) {
		t.Errorf("version_files %v, want %v", p.Release.VersionFiles, want)
	}
	if p := mustLoad(t, minimal); len(p.Release.VersionFiles) != 0 {
		t.Errorf("default version_files %v", p.Release.VersionFiles)
	}
	_, err := pipelineconfig.Load(writeConfig(t, minimal+"  release:\n    version_files: [../x.csproj, \"\", /abs.csproj, a.csproj, a.csproj]\n"))
	for _, want := range []string{"pipeline.release.version_files[0]", "pipeline.release.version_files[1]", "pipeline.release.version_files[2]", `pipeline.release.version_files[4]: duplicate "a.csproj"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q", err, want)
		}
	}
}

func TestLoad_ImageTagsMustIncludeTheVersion(t *testing.T) {
	text := strings.Replace(minimal, "    - name: ghcr.io/acme/app\n", "    - name: ghcr.io/acme/app\n      tags: [channel, latest]\n", 1)
	_, err := pipelineconfig.Load(writeConfig(t, text))
	if want := `pipeline.images[0].tags: must include version (every release has a version tag; channel and latest are not always produced)`; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want %q", err, want)
	}
	ok := strings.Replace(minimal, "    - name: ghcr.io/acme/app\n", "    - name: ghcr.io/acme/app\n      tags: [version]\n", 1)
	if _, err := pipelineconfig.Load(writeConfig(t, ok)); err != nil {
		t.Errorf("tags [version]: %v", err)
	}
}
