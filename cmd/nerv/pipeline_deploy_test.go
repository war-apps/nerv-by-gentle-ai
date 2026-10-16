package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

const pipelineDeployYAML = `pipeline:
  dotnet:
    solution: App.slnx
  images:
    - name: ghcr.io/acme/api
  deploy:
    environments: [production, staging, development]
    ssh:
      known_hosts: ~/.config/update-deploy/known_hosts
    verify:
      timeout: 60s
`

// deployHost fakes the infisical CLI and the Docker hosts behind ssh.
type deployHost struct {
	cmds    []proc.Command
	updates []string
	failOn  string // an environment's container whose update fails
	label   string
}

func (h *deployHost) Run(_ context.Context, c proc.Command) (proc.Result, error) {
	h.cmds = append(h.cmds, c)
	if c.Name == "infisical" {
		path := c.Args[slices.Index(c.Args, "--path")+1]
		env := c.Args[slices.Index(c.Args, "--env")+1]
		if path == "/" {
			return proc.Result{Stdout: []byte(`[{"key":"DOCKER_SSH_HOST","value":"10.9.0.` + env[:1] + `"},{"key":"DOCKER_SSH_USER","value":"deploy"},{"key":"DOCKER_SSH_PASS","value":"inf-s3cr3t"},{"key":"DOCKER_COMPOSE_PATH","value":"/srv"},{"key":"DOCKER_COMPOSE_SCRIPT","value":"./manage.sh"}]`)}, nil
		}
		version := map[string]string{"production": "latest", "staging": "beta", "development": "alpha"}[env]
		return proc.Result{Stdout: []byte(`[{"key":"DOCKER_CONTAINER_NAME","value":"api"},{"key":"DOCKER_CONTAINER_VARIANT","value":"` + env + `"},{"key":"DOCKER_IMAGE_VERSION","value":"` + version + `"}]`)}, nil
	}
	remote := c.Args[len(c.Args)-1]
	switch {
	case strings.HasPrefix(remote, "cd "):
		if h.failOn != "" && strings.Contains(remote, h.failOn) {
			return proc.Result{Stderr: []byte("manage.sh: image not found")}, errors.New("exit status 1")
		}
		h.updates = append(h.updates, remote)
	case strings.HasPrefix(remote, "docker ps"):
		return proc.Result{Stdout: []byte("Up 40 seconds (healthy)\n")}, nil
	case strings.HasPrefix(remote, "docker inspect"):
		return proc.Result{Stdout: []byte(h.label + "\n")}, nil
	}
	return proc.Result{}, nil
}

func (h *deployHost) ssh() []proc.Command {
	var out []proc.Command
	for _, c := range h.cmds {
		if c.Name == "ssh" || c.Name == "sshpass" {
			out = append(out, c)
		}
	}
	return out
}

type deployFixture struct {
	top      string
	host     *deployHost
	env      map[string]string
	terminal bool
	stdin    string
	home     string
}

func deployEnvVars(env, version string) map[string]string {
	p := "NERV_DEPLOY_" + strings.ToUpper(env) + "_"
	return map[string]string{
		p + "DOCKER_SSH_HOST": "10.0.0." + env[:1], p + "DOCKER_SSH_USER": "deploy", p + "DOCKER_SSH_PASS": "s3cr3t-" + env,
		p + "DOCKER_COMPOSE_PATH": "/srv/apps", p + "DOCKER_COMPOSE_SCRIPT": "./manage.sh",
		p + "DOCKER_CONTAINER_NAME": "api", p + "DOCKER_CONTAINER_VARIANT": env, p + "DOCKER_IMAGE_VERSION": version,
	}
}

func newDeployFixture(t *testing.T, yaml string) *deployFixture {
	t.Helper()
	top := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(top, ".nerv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, ".nerv", "nerv.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"GHCR_TOKEN": "registry-secret"}
	for _, m := range []map[string]string{deployEnvVars("production", "latest"), deployEnvVars("staging", "beta"), deployEnvVars("development", "alpha")} {
		for k, v := range m {
			env[k] = v
		}
	}
	return &deployFixture{top: top, host: &deployHost{label: "1.4.0-rc.1"}, env: env, home: t.TempDir()}
}

// release writes release.json for 1.4.0-rc.1 on release/1.4 and the
// docker.json bound to it.
func (f *deployFixture) release(t *testing.T, mod func(*pipeline.ReleaseOutputs)) pipeline.ReleaseOutputs {
	t.Helper()
	o := pipeline.ReleaseOutputs{Step: "release", Status: "passed", Published: true, Branch: "release/1.4", Version: "1.4.0-rc.1", Channel: "beta",
		GitTag: "v1.4.0-rc.1", GitHead: "c0ffee", BaseCommit: "base0"}
	if mod != nil {
		mod(&o)
	}
	if _, err := pipeline.WriteReleaseOutputs(f.top, o); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.WriteDockerResult(f.top, pipeline.DockerResult{Step: "docker", Status: "passed", DryRun: o.DryRun, Version: o.Version, GitTag: o.GitTag, Revision: o.GitHead, Images: []pipeline.ImageResult{}}); err != nil {
		t.Fatal(err)
	}
	return o
}

func (f *deployFixture) run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	opts := testOptions(f.home)
	opts.Runner = &envtest.FakeRunner{Responses: map[string]envtest.Response{"git -C . rev-parse --show-toplevel": {Stdout: f.top + "\n"}}}
	opts.Getenv = func(k string) string { return f.env[k] }
	opts.IsTerminal = func() bool { return f.terminal }
	opts.Stdin = strings.NewReader(f.stdin)
	opts.DeployProc = f.host
	opts.DeploySleep = func(context.Context, time.Duration) error { return nil }
	var out, errOut bytes.Buffer
	code := run(append([]string{"pipeline", "deploy"}, args...), &out, &errOut, opts)
	return code, out.String(), errOut.String()
}

func readDeployJSON(t *testing.T, top string) ([]byte, pipeline.DeployResult, bool) {
	t.Helper()
	data, err := os.ReadFile(pipeline.DeployResultPath(top))
	if errors.Is(err, os.ErrNotExist) {
		return nil, pipeline.DeployResult{}, false
	}
	var r pipeline.DeployResult
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	return data, r, true
}

func statuses(envs []pipeline.EnvironmentDeploy) []string {
	var out []string
	for _, e := range envs {
		out = append(out, e.Environment+":"+e.Status)
	}
	return out
}

func TestPipelineDeploy_DeploysTheEnvironmentsTargetingTheRelease(t *testing.T) {
	f := newDeployFixture(t, pipelineDeployYAML)
	// The deploy data and the registry secrets are in nerv's own process
	// environment too, as in a real run: ssh's environment must not
	// inherit them.
	for k, v := range f.env {
		t.Setenv(k, v)
	}
	t.Setenv("GHCR_USER", "bot")
	t.Setenv("NERV_DEPLOYMENT", "kept")
	f.release(t, nil)
	code, stdout, stderr := f.run(t, "--yes", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	if len(f.host.updates) != 1 || !strings.Contains(f.host.updates[0], "'api-staging'") {
		t.Errorf("updates %q: only staging (beta) targets 1.4.0-rc.1", f.host.updates)
	}
	c := f.host.ssh()[0]
	kh := filepath.Join(f.home, ".config", "update-deploy", "known_hosts")
	n := len(c.Args)
	if c.Name != "sshpass" || !slices.Contains(c.Args, `UserKnownHostsFile="`+kh+`"`) || !slices.Equal(c.Args[n-3:n-1], []string{"--", "10.0.0.s"}) ||
		c.Args[slices.Index(c.Args, "-l")+1] != "deploy" {
		t.Errorf("ssh %s %q", c.Name, c.Args)
	}
	var sshpass []string
	for _, kv := range c.Env {
		switch {
		case strings.HasPrefix(kv, "SSHPASS="):
			sshpass = append(sshpass, kv)
		case strings.HasPrefix(kv, "NERV_DEPLOY_") || strings.HasPrefix(kv, "GHCR_TOKEN=") || strings.HasPrefix(kv, "GHCR_USER="):
			t.Errorf("ssh's environment holds %s", kv)
		}
	}
	if !slices.Equal(sshpass, []string{"SSHPASS=s3cr3t-staging"}) || !slices.Contains(c.Env, "NERV_DEPLOYMENT=kept") || !slices.ContainsFunc(c.Env, func(kv string) bool { return strings.HasPrefix(kv, "PATH=") }) {
		t.Errorf("ssh's environment: SSHPASS %q, the rest kept? %v", sshpass, c.Env)
	}
	_, r, ok := readDeployJSON(t, f.top)
	want := []string{"production:skipped", "staging:deployed", "development:skipped"}
	if !ok || r.Status != "passed" || !slices.Equal(statuses(r.Environments), want) || r.Environments[1].Running != "1.4.0-rc.1" || !strings.Contains(r.Environments[0].Reason, "latest") {
		t.Errorf("deploy.json %+v", r)
	}
	data, _, _ := readDeployJSON(t, f.top)
	for _, secret := range []string{"s3cr3t", "registry-secret"} {
		if strings.Contains(stdout+stderr+string(data), secret) {
			t.Errorf("secret %s printed or recorded", secret)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if !strings.HasPrefix(line, "[pipeline:deploy] ") {
			t.Errorf("unprefixed line %q", line)
		}
	}
}

func TestPipelineDeploy_SelectionByBranches(t *testing.T) {
	f := newDeployFixture(t, strings.Replace(pipelineDeployYAML, "    environments: [production, staging, development]\n",
		"    selection: branches\n    environments:\n      - { name: production, branches: [main] }\n      - { name: development, branches: [develop, \"release/*\"] }\n", 1))
	f.host.label = "1.4.0-rc.1"
	f.release(t, nil)
	if code, _, stderr := f.run(t, "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if len(f.host.updates) != 1 || !strings.Contains(f.host.updates[0], "'api-development'") {
		t.Errorf("updates %q", f.host.updates)
	}
}

func TestPipelineDeploy_FromInfisical(t *testing.T) {
	f := newDeployFixture(t, strings.Replace(pipelineDeployYAML, "    environments:", "    source: infisical\n    infisical: { project_id: p-1, app_path: /api, domain: \"https://infisical.example\" }\n    environments:", 1))
	f.env = map[string]string{}
	f.release(t, nil)
	if code, _, stderr := f.run(t, "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	var inf int
	for _, c := range f.host.cmds {
		if c.Name == "infisical" {
			inf++
			if !slices.Contains(c.Args, "p-1") || !slices.Contains(c.Args, "https://infisical.example") {
				t.Errorf("infisical %q", c.Args)
			}
		}
	}
	if inf != 6 || len(f.host.updates) != 1 || !slices.Contains(f.host.ssh()[0].Env, "SSHPASS=inf-s3cr3t") {
		t.Errorf("infisical calls %d, updates %q", inf, f.host.updates)
	}
}

func TestPipelineDeploy_DryRunConnectsToNothing(t *testing.T) {
	f := newDeployFixture(t, pipelineDeployYAML)
	f.release(t, nil)
	code, _, stderr := f.run(t, "--dry-run")
	if code != 0 || len(f.host.ssh()) != 0 {
		t.Fatalf("exit %d, ssh commands %d\n%s", code, len(f.host.ssh()), stderr)
	}
	if !strings.Contains(stderr, "staging: deploy api-staging on 10.0.0.s") {
		t.Errorf("plan not listed:\n%s", stderr)
	}
	if _, r, ok := readDeployJSON(t, f.top); !ok || !r.DryRun || !slices.Equal(statuses(r.Environments), []string{"production:skipped", "staging:would-deploy", "development:skipped"}) {
		t.Errorf("deploy.json %+v", r)
	}
}

func TestPipelineDeploy_FailureStopsAndIsRecorded(t *testing.T) {
	f := newDeployFixture(t, pipelineDeployYAML)
	f.env["NERV_DEPLOY_DEVELOPMENT_DOCKER_IMAGE_VERSION"] = "beta"
	f.host.failOn = "api-staging"
	f.release(t, nil)
	code, _, stderr := f.run(t, "--yes")
	if code != 2 {
		t.Fatalf("exit %d, want 2\n%s", code, stderr)
	}
	_, r, ok := readDeployJSON(t, f.top)
	if !ok || r.Status != "failed" || !slices.Equal(statuses(r.Environments), []string{"production:skipped", "staging:failed", "development:not-run"}) ||
		!strings.Contains(r.Environments[1].Error, "image not found") || !strings.Contains(r.Action, "no rollback") {
		t.Errorf("deploy.json %+v", r)
	}
}

func TestPipelineDeploy_BlueGreenIsWarned(t *testing.T) {
	f := newDeployFixture(t, pipelineDeployYAML)
	f.env["NERV_DEPLOY_STAGING_DOCKER_DEPLOY_STRATEGY"] = "blue-green"
	f.terminal, f.stdin = true, "y\n"
	f.release(t, nil)
	if code, _, stderr := f.run(t); code != 0 || !strings.Contains(stderr, "api-staging is blue-green") || !strings.Contains(stderr, "[y/N]") {
		t.Errorf("exit %d\n%s", code, stderr)
	}
}

func TestPipelineDeploy_NothingToDo(t *testing.T) {
	for _, tt := range []struct {
		name, yaml string
		mod        func(*pipeline.ReleaseOutputs)
		want       string
	}{
		{"no deploy section", pipelineSyncYAML, nil, "pipeline.deploy is not configured"},
		{"no new release", pipelineDeployYAML, func(o *pipeline.ReleaseOutputs) { o.Published = false }, "no new release"},
		{"no environment targets it", pipelineDeployYAML, func(o *pipeline.ReleaseOutputs) { o.Version, o.GitTag = "1.4.0-next.1", "v1.4.0-next.1" }, "no environment to deploy"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newDeployFixture(t, tt.yaml)
			f.release(t, tt.mod)
			code, _, stderr := f.run(t, "--yes")
			if code != 0 || !strings.Contains(stderr, tt.want) || len(f.host.ssh()) != 0 {
				t.Errorf("exit %d ssh %d\n%s", code, len(f.host.ssh()), stderr)
			}
		})
	}
}

func TestPipelineDeploy_Refusals(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*testing.T, *deployFixture)
		args  []string
		want  string
	}{
		{"no release.json", func(*testing.T, *deployFixture) {}, nil, "run nerv pipeline release first"},
		{"failed release", func(t *testing.T, f *deployFixture) {
			f.release(t, func(o *pipeline.ReleaseOutputs) { o.Status = "failed" })
		}, nil, "did not pass"},
		{"dry-run release", func(t *testing.T, f *deployFixture) {
			f.release(t, func(o *pipeline.ReleaseOutputs) { o.DryRun = true })
		}, nil, "dry run"},
		{"no docker.json", func(t *testing.T, f *deployFixture) {
			f.release(t, nil)
			os.Remove(pipeline.DockerResultPath(f.top))
		}, nil, "docker: no docker.json"},
		{"stale docker.json", func(t *testing.T, f *deployFixture) {
			f.release(t, nil)
			pipeline.WriteDockerResult(f.top, pipeline.DockerResult{Status: "passed", Version: "1.3.0", GitTag: "v1.3.0", Revision: "old"})
		}, nil, "not this release"},
		{"missing DOCKER_IMAGE_VERSION", func(t *testing.T, f *deployFixture) {
			f.release(t, nil)
			delete(f.env, "NERV_DEPLOY_PRODUCTION_DOCKER_IMAGE_VERSION")
		}, nil, "production: DOCKER_IMAGE_VERSION is not set"},
		{"deployed environment without host", func(t *testing.T, f *deployFixture) {
			f.release(t, nil)
			delete(f.env, "NERV_DEPLOY_STAGING_DOCKER_SSH_HOST")
		}, nil, "staging: missing DOCKER_SSH_HOST"},
		{"no terminal", func(t *testing.T, f *deployFixture) { f.release(t, nil) }, []string{}, "pass --yes"},
		{"declined", func(t *testing.T, f *deployFixture) {
			f.release(t, nil)
			f.terminal, f.stdin = true, "n\n"
		}, []string{}, "cancelled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newDeployFixture(t, pipelineDeployYAML)
			earlier := pipeline.DeployResult{Step: "deploy", Status: "passed", Version: "1.3.0", Environments: []pipeline.EnvironmentDeploy{}}
			if _, err := pipeline.WriteDeployResult(f.top, earlier); err != nil {
				t.Fatal(err)
			}
			before, _, _ := readDeployJSON(t, f.top)
			tt.setup(t, f)
			args := tt.args
			if args == nil {
				args = []string{"--yes"}
			}
			code, _, stderr := f.run(t, args...)
			if code != 1 || !strings.Contains(stderr, tt.want) {
				t.Errorf("exit %d (want 1), stderr:\n%s", code, stderr)
			}
			if len(f.host.ssh()) != 0 {
				t.Errorf("ssh ran: %d commands", len(f.host.ssh()))
			}
			if after, _, _ := readDeployJSON(t, f.top); !bytes.Equal(after, before) {
				t.Errorf("deploy.json changed by a refusal:\n%s", after)
			}
		})
	}
}
