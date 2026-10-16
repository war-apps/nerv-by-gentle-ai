package pipeline

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// fakeDocker records docker commands; fail decides a command's outcome.
type fakeDocker struct {
	cmds []proc.Command
	fail func(c proc.Command) error
	out  func(c proc.Command) string
}

func (f *fakeDocker) Run(_ context.Context, c proc.Command) (proc.Result, error) {
	f.cmds = append(f.cmds, c)
	res := proc.Result{}
	if f.out != nil {
		res.Stdout = []byte(f.out(c))
	}
	if f.fail != nil {
		return res, f.fail(c)
	}
	return res, nil
}

func (f *fakeDocker) argvs(sub string) [][]string {
	var out [][]string
	for _, c := range f.cmds {
		if len(c.Args) > 0 && c.Args[0] == sub {
			out = append(out, c.Args)
		}
	}
	return out
}

// fakeArgs answers computed build arguments: one value per call.
type fakeArgs struct {
	values []string
	calls  int
	err    error
}

func (f *fakeArgs) Capture(context.Context, shiprun.Step, shiprun.Vars) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.values[min(f.calls, len(f.values))-1] + "\n", nil
}

func dockerConfig() *pipelineconfig.Pipeline {
	no := false
	return &pipelineconfig.Pipeline{
		Branches: pipelineconfig.DefaultBranches(),
		Registry: pipelineconfig.Registry{Host: "ghcr.io"},
		Images: []pipelineconfig.Image{
			{Name: "ghcr.io/acme/api", Title: "acme.api", Dockerfile: "docker/Dockerfile", Context: ".", Platform: "linux/amd64",
				BuildArgs: map[string]string{"APP_VERSION": "${version}", "NOTE": "${channel}/${tag}"}, Tags: []string{"version", "channel", "latest"}},
			{Name: "ghcr.io/acme/api-login-broker", Title: "acme.login-broker", Dockerfile: "docker/LoginBroker.Dockerfile", Context: ".", Platform: "linux/amd64",
				ComputedArgs: map[string]pipelineconfig.ComputedArg{"CHROME_VERSION": {Run: "resolve-chrome", Shell: "bash"}}, Tags: []string{"version", "channel", "latest"}, Required: &no},
		},
	}
}

func release(branch, version, channel, tag string) ReleaseOutputs {
	return ReleaseOutputs{Step: "release", Status: StatusPassed, Published: true, Branch: branch, Version: version, Channel: channel, GitTag: tag, GitHead: "c0ffee"}
}

func TestImageTags(t *testing.T) {
	all := []string{"version", "channel", "latest"}
	tests := []struct {
		name, branch, version, channel string
		sel                            []string
		want                           []string
	}{
		{"main", "main", "1.4.0", "", all, []string{"1.4.0", "latest"}},
		{"main with the default channel", "main", "1.4.0", "default", all, []string{"1.4.0", "latest"}},
		{"develop", "develop", "1.4.0-alpha.3", "alpha", all, []string{"1.4.0-alpha.3", "alpha"}},
		{"release/*", "release/1.4", "1.4.0-rc.1", "beta", all, []string{"1.4.0-rc.1", "beta"}},
		{"hotfix", "hotfix/auth", "1.4.1-beta.1", "beta", all, []string{"1.4.1-beta.1", "beta"}},
		{"selection", "main", "1.4.0", "", []string{"version"}, []string{"1.4.0"}},
	}
	cfg := dockerConfig()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ImageTags(tt.sel, tt.version, tt.channel, tt.branch == cfg.MainBranch())
			if !slices.Equal(got, tt.want) {
				t.Errorf("tags %v, want %v", got, tt.want)
			}
		})
	}
}

func dockerStep(f *fakeDocker, a *fakeArgs, rel ReleaseOutputs) Docker {
	return Docker{Dir: "/clone", Config: dockerConfig(), Release: rel, Revision: "c0ffee", Source: "https://github.com/acme/api", Proc: f, Args: a, Output: &strings.Builder{}}
}

func TestDocker_BuildsAndPushesEveryImage(t *testing.T) {
	f := &fakeDocker{out: func(c proc.Command) string {
		if c.Args[0] == "push" {
			return "1.4.0-alpha.3: digest: sha256:" + strings.Repeat("a", 64) + " size: 528\n"
		}
		return ""
	}}
	a := &fakeArgs{values: []string{"130.0.1"}}
	d := dockerStep(f, a, release("develop", "1.4.0-alpha.3", "alpha", "v1.4.0-alpha.3"))
	d.Login = &Credentials{Host: "ghcr.io", User: "bot", Token: "s3cr3t-token"}
	r := d.Run(context.Background())
	if r.Status != StatusPassed || len(r.Images) != 2 {
		t.Fatalf("result %+v", r)
	}
	builds := f.argvs("build")
	want := []string{"build", "--platform", "linux/amd64", "-f", "docker/Dockerfile",
		"--build-arg", "APP_VERSION=1.4.0-alpha.3", "--build-arg", "NOTE=alpha/v1.4.0-alpha.3",
		"--label", "org.opencontainers.image.title=acme.api", "--label", "org.opencontainers.image.version=1.4.0-alpha.3",
		"--label", "org.opencontainers.image.revision=c0ffee", "--label", "org.opencontainers.image.source=https://github.com/acme/api",
		"-t", "ghcr.io/acme/api:1.4.0-alpha.3", "-t", "ghcr.io/acme/api:alpha", "."}
	if !slices.Equal(builds[0], want) {
		t.Errorf("build argv\n %q\nwant\n %q", builds[0], want)
	}
	if !slices.Contains(builds[1], "CHROME_VERSION=130.0.1") {
		t.Errorf("broker build %q lacks the computed arg", builds[1])
	}
	pushes := f.argvs("push")
	if len(pushes) != 4 || pushes[0][1] != "ghcr.io/acme/api:1.4.0-alpha.3" || pushes[1][1] != "ghcr.io/acme/api:alpha" {
		t.Errorf("pushes %q", pushes)
	}
	if img := r.Images[0]; img.Status != "pushed" || img.Digest != "sha256:"+strings.Repeat("a", 64) || img.Attempts != 1 || !slices.Equal(img.Tags, []string{"1.4.0-alpha.3", "alpha"}) {
		t.Errorf("image %+v", img)
	}
	// Login first, the token on stdin only.
	login := f.cmds[0]
	if !slices.Equal(login.Args, []string{"login", "ghcr.io", "-u", "bot", "--password-stdin"}) || string(login.Stdin) != "s3cr3t-token\n" {
		t.Errorf("login %q stdin %q", login.Args, login.Stdin)
	}
	for _, c := range f.cmds {
		if strings.Contains(strings.Join(c.Args, " "), "s3cr3t") || slices.ContainsFunc(c.Env, func(kv string) bool { return strings.Contains(kv, "s3cr3t") }) {
			t.Errorf("token outside stdin: %q", c.Args)
		}
		if c.Dir != "/clone" {
			t.Errorf("%q runs in %s, want the release clone", c.Args, c.Dir)
		}
	}
	if strings.Contains(d.Output.(*strings.Builder).String(), "s3cr3t") {
		t.Error("token printed")
	}
}

func TestDocker_RetriesOnceWithRecomputedArgs(t *testing.T) {
	builds := 0
	f := &fakeDocker{fail: func(c proc.Command) error {
		if c.Args[0] == "build" && slices.Contains(c.Args, "-f") && c.Args[4] == "docker/LoginBroker.Dockerfile" {
			builds++
			if builds == 1 {
				return errors.New("exit status 100")
			}
		}
		return nil
	}}
	a := &fakeArgs{values: []string{"130.0.1", "130.0.2"}}
	r := dockerStep(f, a, release("main", "1.4.0", "", "v1.4.0")).Run(context.Background())
	broker := r.Images[1]
	if r.Status != StatusPassed || broker.Status != "pushed" || broker.Attempts != 2 || a.calls != 2 {
		t.Fatalf("result %+v, broker %+v, arg calls %d", r, broker, a.calls)
	}
	b := f.argvs("build")
	if !slices.Contains(b[1], "CHROME_VERSION=130.0.1") || !slices.Contains(b[2], "CHROME_VERSION=130.0.2") {
		t.Errorf("retry did not recompute: %q / %q", b[1], b[2])
	}
	if !slices.Contains(b[0], "ghcr.io/acme/api:latest") {
		t.Errorf("main build lacks latest: %q", b[0])
	}
}

func TestDocker_NotRequiredImageMayFail(t *testing.T) {
	f := &fakeDocker{fail: func(c proc.Command) error {
		if c.Args[0] == "build" && c.Args[4] == "docker/LoginBroker.Dockerfile" {
			return errors.New("exit status 1")
		}
		return nil
	}}
	r := dockerStep(f, &fakeArgs{values: []string{"1", "2"}}, release("main", "1.4.0", "", "v1.4.0")).Run(context.Background())
	if r.Status != StatusPassed || r.Images[1].Status != "failed" || r.Images[1].Attempts != 2 || r.Images[1].Error == "" || r.Images[0].Status != "pushed" {
		t.Errorf("result %+v", r)
	}
	if len(f.argvs("push")) != 2 {
		t.Errorf("pushes %q: only the api image", f.argvs("push"))
	}
}

func TestDocker_RequiredImageFailureFailsTheStepButOthersStillRun(t *testing.T) {
	f := &fakeDocker{fail: func(c proc.Command) error {
		if c.Args[0] == "push" && strings.HasSuffix(c.Args[1], ":latest") && strings.Contains(c.Args[1], "/api:") {
			return errors.New("denied")
		}
		return nil
	}}
	r := dockerStep(f, &fakeArgs{values: []string{"1"}}, release("main", "1.4.0", "", "v1.4.0")).Run(context.Background())
	if r.Status != StatusFailed || r.Images[0].Status != "failed" || !strings.Contains(r.Images[0].Error, "push ghcr.io/acme/api:latest") || r.Images[1].Status != "pushed" || !strings.Contains(r.Error, "ghcr.io/acme/api") {
		t.Errorf("result %+v", r)
	}
	// A failed push is not retried by rebuilding.
	if r.Images[0].Attempts != 1 {
		t.Errorf("attempts %d", r.Images[0].Attempts)
	}
}

func TestDocker_DryRunBuildsButNeverLogsInOrPushes(t *testing.T) {
	f := &fakeDocker{}
	d := dockerStep(f, &fakeArgs{values: []string{"1"}}, release("main", "1.4.0", "", "v1.4.0"))
	d.DryRun = true
	d.Login = &Credentials{Host: "ghcr.io", User: "bot", Token: "x"}
	r := d.Run(context.Background())
	if r.Status != StatusPassed || len(f.argvs("build")) != 2 || len(f.argvs("push")) != 0 || len(f.argvs("login")) != 0 || r.Images[0].Status != "built" || !r.DryRun {
		t.Errorf("result %+v, commands %d", r, len(f.cmds))
	}
}

func TestDocker_LoginFailureStopsBeforeBuilding(t *testing.T) {
	f := &fakeDocker{fail: func(c proc.Command) error {
		if c.Args[0] == "login" {
			return errors.New("exit status 1")
		}
		return nil
	}}
	d := dockerStep(f, &fakeArgs{}, release("main", "1.4.0", "", "v1.4.0"))
	d.Login = &Credentials{Host: "ghcr.io", User: "bot", Token: "x"}
	r := d.Run(context.Background())
	if r.Status != StatusFailed || len(f.argvs("build")) != 0 || !strings.Contains(r.Error, "docker login ghcr.io") {
		t.Errorf("result %+v", r)
	}
}

func TestDocker_ChildrenNeverInheritTheRegistrySecrets(t *testing.T) {
	t.Setenv("GHCR_USER", "bot-env")
	t.Setenv("GHCR_TOKEN", "s3cr3t-env-token")
	t.Setenv("NERV_DEPLOY_PRODUCTION_DOCKER_SSH_KEY", "deploy-secret")
	t.Setenv("DOCKER_CONFIG", "/home/me/.docker")
	t.Setenv("HTTPS_PROXY", "http://proxy:3128")
	f := &fakeDocker{}
	d := dockerStep(f, &fakeArgs{values: []string{"1"}}, release("main", "1.4.0", "", "v1.4.0"))
	d.Config.Registry.UsernameEnv, d.Config.Registry.TokenEnv = "GHCR_USER", "GHCR_TOKEN"
	d.Login = &Credentials{Host: "ghcr.io", User: "bot-env", Token: "s3cr3t-env-token"}
	if r := d.Run(context.Background()); r.Status != StatusPassed {
		t.Fatalf("result %+v", r)
	}
	if len(f.cmds) < 3 {
		t.Fatalf("commands %d", len(f.cmds))
	}
	for _, c := range f.cmds {
		for _, kv := range c.Env {
			name, _, _ := strings.Cut(kv, "=")
			if name == "GHCR_TOKEN" || name == "GHCR_USER" || strings.HasPrefix(name, "NERV_DEPLOY_") ||
				strings.Contains(kv, "s3cr3t-env-token") || strings.Contains(kv, "deploy-secret") {
				t.Errorf("%q: secret in the environment: %s", c.Args, name)
			}
		}
		if strings.Contains(strings.Join(c.Args, " "), "s3cr3t-env-token") {
			t.Errorf("token on argv: %q", c.Args)
		}
		for _, keep := range []string{"DOCKER_CONFIG=/home/me/.docker", "HTTPS_PROXY=http://proxy:3128"} {
			if !slices.Contains(c.Env, keep) {
				t.Errorf("%q lost %s", c.Args, keep)
			}
		}
		if !slices.ContainsFunc(c.Env, func(kv string) bool { return strings.HasPrefix(kv, "PATH=") }) {
			t.Errorf("%q lost PATH", c.Args)
		}
	}
	if string(f.cmds[0].Stdin) != "s3cr3t-env-token\n" {
		t.Errorf("login stdin %q", f.cmds[0].Stdin)
	}
}

func TestSafeEnv(t *testing.T) {
	env := []string{"PATH=/bin", "GHCR_TOKEN=t", "ghcr_user=u", "NERV_DEPLOY_STAGING_DOCKER_SSH_HOST=h", "NERV_DEPLOYMENT=keep", "DOCKER_HOST=unix:///x"}
	got := SafeEnv(env, pipelineconfig.Registry{UsernameEnv: "GHCR_USER", TokenEnv: "GHCR_TOKEN"})
	if want := []string{"PATH=/bin", "NERV_DEPLOYMENT=keep", "DOCKER_HOST=unix:///x"}; !slices.Equal(got, want) {
		t.Errorf("SafeEnv = %v, want %v", got, want)
	}
}

func TestDocker_AnImageWithNoTagsFailsWithoutBuilding(t *testing.T) {
	f := &fakeDocker{}
	d := dockerStep(f, &fakeArgs{values: []string{"1"}}, release("main", "1.4.0", "", "v1.4.0"))
	d.Config.Images[0].Tags = []string{"channel"} // main has no channel
	r := d.Run(context.Background())
	img := r.Images[0]
	if r.Status != StatusFailed || img.Status != "failed" || !strings.Contains(img.Error, "no tag") || img.Attempts != 0 {
		t.Errorf("result %+v, image %+v", r, img)
	}
	for _, b := range f.argvs("build") {
		if b[4] == "docker/Dockerfile" {
			t.Errorf("built an image without a tag: %q", b)
		}
	}
}

func TestDocker_AFailedBuildReportsItsCause(t *testing.T) {
	f := &fakeDocker{
		out: func(c proc.Command) string {
			if c.Args[0] == "build" {
				return "#13 4.571 restoring\n#13 5.774 error NU1903: Package 'Pkcs' 9.0.4 has a known high severity vulnerability\nERROR: failed to solve: dotnet restore did not complete successfully: exit code: 1\n"
			}
			return ""
		},
		fail: func(c proc.Command) error {
			if c.Args[0] == "build" {
				return errors.New("exit status 1")
			}
			return nil
		},
	}
	r := dockerStep(f, &fakeArgs{values: []string{"130.0"}}, release("main", "2.9.0", "", "v2.9.0")).Run(context.Background())
	if r.Status != StatusFailed {
		t.Fatalf("status %s", r.Status)
	}
	name := dockerConfig().Images[0].Name
	for _, want := range []string{name, "build " + name, "exit status 1", "error NU1903", "failed to solve"} {
		if !strings.Contains(r.Error, want) {
			t.Errorf("error %q lacks %q", r.Error, want)
		}
	}
	if !strings.Contains(r.Images[0].Error, "error NU1903") {
		t.Errorf("image error %q lacks the build's last lines", r.Images[0].Error)
	}
}
