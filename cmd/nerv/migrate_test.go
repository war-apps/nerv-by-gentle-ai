package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

// migrateRepo is a repository holding a 4.2.0 fixture of
// internal/migrate/testdata, and the options running nerv against it.
func migrateRepo(t *testing.T, fixture string) (top string, opts options) {
	t.Helper()
	top = filepath.Join(t.TempDir(), "repo")
	src := filepath.Join("..", "..", "internal", "migrate", "testdata", fixture)
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(top, e.Name())
		if e.Name() == "nerv.yaml" {
			dst = filepath.Join(top, ".nerv", "nerv.yaml")
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts = testOptions(t.TempDir())
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.Runner = &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"git -C . rev-parse --show-toplevel":       {Stdout: top + "\n"},
		"git -C " + top + " remote get-url origin": {Stdout: "git@github.com:dirm-s-a/dim.comprobantes.api.git\n"},
	}}
	return top, opts
}

func runMigrateCLI(t *testing.T, opts options, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{"migrate"}, args...), &out, &errOut, opts)
	return code, out.String(), errOut.String()
}

func TestMigrate_DryRunPrintsEveryChangeAndWritesNothing(t *testing.T) {
	top, opts := migrateRepo(t, "dim.comprobantes.api")
	code, out, errOut := runMigrateCLI(t, opts, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	for _, want := range []string{
		"--- /dev/null\n+++ b/.ai-workflow.yml\n", "+ship:", "+deploy:\n+  host: github",
		"move .nerv/nerv.yaml -> .nerv/nerv.yaml.migrated",
		"warning: semantic-release package.json: @semantic-release/exec",
		"plugins: nerv install --with task,task-teamwork,ship,deploy,deploy-github",
		"nerv migrate: dry run:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(top, ".ai-workflow.yml")); err == nil {
		t.Error("--dry-run wrote .ai-workflow.yml")
	}
	if _, err := os.Stat(filepath.Join(top, ".nerv", "nerv.yaml")); err != nil {
		t.Errorf("--dry-run moved .nerv/nerv.yaml: %v", err)
	}
}

func TestMigrate_WritesThenASecondRunHasNothingToDo(t *testing.T) {
	top, opts := migrateRepo(t, "dim-factura-electronica-service")
	code, out, errOut := runMigrateCLI(t, opts)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if !strings.Contains(out, "backup: "+filepath.Join(top, ".nerv", "backups")) || !strings.Contains(out, "change(s) written") {
		t.Errorf("stdout:\n%s", out)
	}
	code, out, _ = runMigrateCLI(t, opts)
	if code != 0 || !strings.Contains(out, "nerv migrate: nothing to migrate") {
		t.Errorf("second run: exit %d\n%s", code, out)
	}
}

func TestMigrate_OutsideARepositoryRefusesUnlessUser(t *testing.T) {
	opts := testOptions(t.TempDir())
	opts.ExtensionsFS = nerv.ExtensionsFS()
	opts.Runner = &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"git -C . rev-parse --show-toplevel": {Stderr: "fatal: not a git repository", ExitCode: 128},
	}}
	if code, out, errOut := runMigrateCLI(t, opts); code != 1 || !strings.Contains(errOut, "not inside a git repository") {
		t.Errorf("exit %d\n%s\n%s", code, out, errOut)
	}
	code, out, errOut := runMigrateCLI(t, opts, "--user")
	if code != 0 || !strings.Contains(out, "nothing to migrate") {
		t.Errorf("--user outside a repository: exit %d\n%s\n%s", code, out, errOut)
	}
}

func TestMigrate_UsageErrors(t *testing.T) {
	opts := testOptions(t.TempDir())
	for _, args := range [][]string{{"--nope"}, {"extra"}} {
		if code, _, _ := runMigrateCLI(t, opts, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
	if code, out, _ := runMigrateCLI(t, opts, "--help"); code != 0 || !strings.Contains(out, "Usage: nerv migrate") {
		t.Errorf("--help: exit %d\n%s", code, out)
	}
}

func TestMigrate_InstallRunsTheInstallStep(t *testing.T) {
	_, opts := migrateRepo(t, "dim.comprobantes.api")
	seedShipFamilyForCLI(t, opts.Home)
	runner := opts.Runner.(*envtest.FakeRunner)
	for k, v := range baseInstallRunner().Responses {
		runner.Responses[k] = v
	}
	code, out, errOut := runMigrateCLI(t, opts, "--install")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	for _, plugin := range []string{"task@nerv", "task-teamwork@nerv", "ship@nerv", "deploy@nerv", "deploy-github@nerv"} {
		if !strings.Contains(out, "claude plugin install "+plugin) {
			t.Errorf("%s not installed:\n%s", plugin, out)
		}
	}
	if strings.Contains(out, "claude plugin install deploy-ssh-compose@nerv") {
		t.Errorf("deploy-ssh-compose installed, but the repository never used pipeline.deploy:\n%s", out)
	}
}

// oldDockerArgv is what the 4.2.0 docker preset ran for dim.comprobantes.api
// (internal/shipplan/presets.go at 03d4401): one build per image with the
// version tag, the channel tag off the default channel, latest on it, the
// sorted build arguments and the version and revision labels.
func oldDockerArgv(version, channel, head string) [][]string {
	var out [][]string
	for _, img := range []struct{ name, dockerfile string }{
		{"ghcr.io/dirm-s-a/dim.comprobantes.api", "docker/Dockerfile"},
		{"ghcr.io/dirm-s-a/dim.comprobantes.api-login-broker", "docker/LoginBroker.Dockerfile"},
	} {
		argv := []string{"docker", "build", "-f", img.dockerfile, "-t", img.name + ":" + version}
		if channel == "" {
			argv = append(argv, "-t", img.name+":latest")
		} else {
			argv = append(argv, "-t", img.name+":"+channel)
		}
		argv = append(argv, "--build-arg", "APP_VERSION="+version, "--label", "org.opencontainers.image.version="+version,
			"--label", "org.opencontainers.image.revision="+head, ".")
		out = append(out, argv)
	}
	return out
}

// evalDocker runs a stage script under bash with docker replaced by a
// function printing its argv, and returns the docker build calls.
func evalDocker(t *testing.T, script string, env map[string]string) [][]string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available")
	}
	cmd := exec.Command(bash, "--noprofile", "--norc", "-e", "-o", "pipefail", "-c",
		`docker() { local a; for a in "$@"; do printf '%s\037' "$a"; done; printf '\n'; }`+"\n"+script)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the stage script failed: %v\n%s", err, out)
	}
	var builds [][]string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		argv := append([]string{"docker"}, strings.Split(strings.TrimSuffix(line, "\037"), "\037")...)
		if argv[1] == "build" {
			builds = append(builds, argv)
		}
	}
	return builds
}

// nerv ship --dry-run of the migrated dim.comprobantes.api runs the commands
// 4.2.0 ran: its own stage commands verbatim, and the docker preset's
// builds; ship then saves build.images into the artifact. The push moved
// to deploy.
func TestMigrate_ShipDryRunPlansThe420Commands(t *testing.T) {
	top, opts := migrateRepo(t, "dim.comprobantes.api")
	old, err := os.ReadFile(filepath.Join(top, ".nerv", "nerv.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := migrate.Compute(migrate.Options{RepoRoot: top, ExtensionsFS: opts.ExtensionsFS, OriginURL: "https://github.com/dirm-s-a/dim.comprobantes.api.git", Now: opts.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var migrated string
	for _, c := range plan.Changes {
		if filepath.Base(c.Path) == ".ai-workflow.yml" {
			migrated = string(c.New)
		}
	}
	f := newShipFixture(t, migrated)
	csproj := filepath.Join(f.ws.dir, "src", "Api", "Dim.Comprobantes.Api.csproj")
	if err := os.MkdirAll(filepath.Dir(csproj), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(csproj, []byte("<Project><PropertyGroup><Version>1.0.0</Version></PropertyGroup></Project>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.ws.results = []versioning.Result{shipResult("api", "1.2.0", "v1.2.0")}
	code, stdout, stderr := f.run(t, "--dry-run")
	if code != 0 {
		t.Fatalf("nerv ship --dry-run: exit %d\n%s\n%s", code, stdout, stderr)
	}
	stages := map[string]shiprun.Step{}
	var order, saves []string
	for _, s := range f.steps {
		if _, seen := stages[s.Stage]; !seen {
			stages[s.Stage] = s
			order = append(order, s.Stage)
			continue
		}
		if s.Stage == "package" {
			saves = append(saves, strings.Join(s.Exec, " "))
		}
	}
	if want := []string{"restore", "lint", "build", "test", "coverage", "package"}; !slices.Equal(order, want) {
		t.Fatalf("stages run %q, want %q", order, want)
	}
	// L22: after the package commands, ship saves build.images into the
	// artifact itself (and records them for deploy to push).
	outputs := filepath.Join(f.ws.ArtifactsDir(), "api", "outputs")
	if want := []string{
		"docker save -o " + filepath.Join(outputs, "ghcr.io_dirm-s-a_dim.comprobantes.api_1.2.0.tar") + " ghcr.io/dirm-s-a/dim.comprobantes.api:1.2.0",
		"docker save -o " + filepath.Join(outputs, "ghcr.io_dirm-s-a_dim.comprobantes.api-login-broker_1.2.0.tar") + " ghcr.io/dirm-s-a/dim.comprobantes.api-login-broker:1.2.0",
	}; !slices.Equal(saves, want) {
		t.Errorf("image saves %q, want %q", saves, want)
	}
	for _, stage := range []string{"restore", "lint", "build", "test", "coverage"} {
		if !strings.Contains(string(old), "run: "+stages[stage].Command.Run+"\n") {
			t.Errorf("stage %s ran %q, not its 4.2.0 command", stage, stages[stage].Command.Run)
		}
	}
	for _, channel := range []string{"", "alpha"} {
		version := "1.2.0"
		if channel != "" {
			version = "1.2.0-alpha.3"
		}
		env := map[string]string{
			"NERV_RELEASE_VERSION": version, "NERV_RELEASE_CHANNEL": channel, "NERV_RELEASE_TAG": "v" + version,
			"NERV_RELEASE_GIT_HEAD": localHead, "NERV_RELEASE_OUTPUTS_DIR": t.TempDir(),
		}
		got := evalDocker(t, stages["package"].Command.Run, env)
		want := oldDockerArgv(version, channel, localHead)
		if len(got) != len(want) {
			t.Fatalf("channel %q: %d docker builds, want %d: %q", channel, len(got), len(want), got)
		}
		for i := range want {
			if !slices.Equal(got[i], want[i]) {
				t.Errorf("channel %q: build %d\n got %q\nwant %q", channel, i, got[i], want[i])
			}
		}
	}
	if strings.Contains(stages["package"].Command.Run, "docker save") {
		t.Errorf("the package stage saves the pushed images itself (ship does):\n%s", stages["package"].Command.Run)
	}
}
