package deploy

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

func remoteOf(c proc.Command) string { return c.Args[len(c.Args)-1] }

// host fakes a Docker host: the update succeeds, docker ps answers the next
// status of statuses (the last one repeats) and the label is label.
type host struct {
	statuses []string
	label    string
	failOn   string // a remote command prefix that fails
	updates  []string
}

func (h *host) answer(c proc.Command) (proc.Result, error) {
	r := remoteOf(c)
	if h.failOn != "" && strings.HasPrefix(r, h.failOn) {
		return proc.Result{Stderr: []byte("manage.sh: pull failed")}, errors.New("exit status 1")
	}
	switch {
	case strings.HasPrefix(r, "cd "):
		h.updates = append(h.updates, r)
	case strings.HasPrefix(r, "docker ps"):
		s := h.statuses[0]
		if len(h.statuses) > 1 {
			h.statuses = h.statuses[1:]
		}
		return proc.Result{Stdout: []byte(s + "\n")}, nil
	case strings.HasPrefix(r, "docker inspect"):
		return proc.Result{Stdout: []byte(h.label + "\n")}, nil
	}
	return proc.Result{}, nil
}

func passwordSSH(f *fakeProc) SSH {
	return SSH{Proc: f, Auth: "password", KnownHosts: "/kh", Port: 22, ConnectTimeout: 10 * time.Second, Env: []string{"PATH=/bin"}}
}

func target(env string) Target {
	return Target{Environment: env, Host: "10.0.0.1", User: "deploy", Password: "s3cr3t", ComposePath: "/srv/my apps", ComposeScript: "./manage.sh",
		Container: "api-" + env, Variant: env, ImageVersion: "1.4.0", Enabled: true}
}

func newRunner(f *fakeProc, healthy bool) (*Runner, *int) {
	slept := 0
	yes := healthy
	return &Runner{
		SSH:     passwordSSH(f),
		Verify:  pipelineconfig.Verify{Healthy: &yes, VersionLabel: "org.opencontainers.image.version", Timeout: "60s"},
		Release: pipeline.ReleaseOutputs{Version: "1.4.0", GitTag: "v1.4.0"},
		Sleep:   func(context.Context, time.Duration) error { slept++; return nil },
		Now:     func() time.Time { return time.Unix(int64(slept)*5, 0) },
	}, &slept
}

func TestSSH_PasswordGoesThroughSSHPASSOnly(t *testing.T) {
	f := &fakeProc{}
	if _, err := passwordSSH(f).Run(context.Background(), target("staging"), "uptime", nil); err != nil {
		t.Fatal(err)
	}
	c := f.cmds[0]
	want := []string{"-e", "ssh", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=/kh", "-o", "ConnectTimeout=10", "-o", "LogLevel=ERROR",
		"-p", "22", "-o", "PreferredAuthentications=password", "-o", "PubkeyAuthentication=no", "-o", "NumberOfPasswordPrompts=1", "deploy@10.0.0.1", "uptime"}
	if c.Name != "sshpass" || !slices.Equal(c.Args, want) {
		t.Errorf("command %s %q", c.Name, c.Args)
	}
	if !slices.Contains(c.Env, "SSHPASS=s3cr3t") || !slices.Contains(c.Env, "PATH=/bin") || strings.Contains(strings.Join(c.Args, " "), "s3cr3t") {
		t.Errorf("env %q args %q: the password goes in SSHPASS only", c.Env, c.Args)
	}
}

func TestSSH_KeyAuth(t *testing.T) {
	f := &fakeProc{}
	s := passwordSSH(f)
	s.Auth, s.KeyFile, s.Port = "key", "/home/u/.ssh/deploy", 2222
	if _, err := s.Run(context.Background(), target("staging"), "uptime", nil); err != nil {
		t.Fatal(err)
	}
	c := f.cmds[0]
	if c.Name != "ssh" || !slices.Contains(c.Args, "/home/u/.ssh/deploy") || !slices.Contains(c.Args, "BatchMode=yes") || !slices.Contains(c.Args, "IdentitiesOnly=yes") ||
		!slices.Contains(c.Args, "2222") || slices.ContainsFunc(c.Env, func(kv string) bool { return strings.HasPrefix(kv, "SSHPASS=") }) {
		t.Errorf("command %s %q env %q", c.Name, c.Args, c.Env)
	}
}

func TestSSH_UnknownHostKeyIsExplained(t *testing.T) {
	f := &fakeProc{answer: func(proc.Command) (proc.Result, error) { return proc.Result{}, exitErr(6) }}
	_, err := passwordSSH(f).Run(context.Background(), target("staging"), "uptime", nil)
	if err == nil || !strings.Contains(err.Error(), "known_hosts") || !strings.Contains(err.Error(), "/kh") || strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("err = %v", err)
	}
}

type exitErr int

func (e exitErr) Error() string { return "exit status " + string(rune('0'+int(e))) }
func (e exitErr) ExitCode() int { return int(e) }

func TestRunner_UpdatesWaitsForHealthAndChecksTheVersion(t *testing.T) {
	h := &host{statuses: []string{"Up 2 seconds (health: starting)", "Up 30 seconds (health: starting)", "Up 40 seconds (healthy)"}, label: "v1.4.0"}
	f := &fakeProc{answer: h.answer}
	r, slept := newRunner(f, true)
	res := r.Deploy(context.Background(), target("staging"))
	if res.Status != pipeline.DeployDeployed || res.Running != "v1.4.0" || res.Health != "Up 40 seconds (healthy)" || *slept != 2 {
		t.Fatalf("result %+v, slept %d", res, *slept)
	}
	wantUpdate := `cd '/srv/my apps' && './manage.sh' pull --container 'api-staging' && './manage.sh' up --container 'api-staging'`
	if len(h.updates) != 1 || h.updates[0] != wantUpdate {
		t.Errorf("update %q, want %q", h.updates, wantUpdate)
	}
	ps := remoteOf(f.cmds[1])
	if ps != `docker ps --all --filter 'name=^api-staging$' --format '{{.Status}}'` {
		t.Errorf("docker ps %q", ps)
	}
	if label := remoteOf(f.cmds[len(f.cmds)-1]); label != `docker inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' 'api-staging'` {
		t.Errorf("inspect %q", label)
	}
}

func TestRunner_Failures(t *testing.T) {
	for _, tt := range []struct {
		name    string
		host    host
		healthy bool
		want    string
	}{
		{"update fails", host{failOn: "cd ", statuses: []string{"Up"}}, true, "manage.sh: pull failed"},
		{"unhealthy", host{statuses: []string{"Up 1 minute (unhealthy)"}, label: "1.4.0"}, true, "unhealthy"},
		{"never healthy", host{statuses: []string{"Up 5 seconds (health: starting)"}, label: "1.4.0"}, true, "not healthy after 1m0s"},
		{"no container", host{statuses: []string{""}, label: "1.4.0"}, true, "no container api-staging"},
		{"exited", host{statuses: []string{"Exited (1) 3 seconds ago"}, label: "1.4.0"}, false, "not running"},
		{"other version", host{statuses: []string{"Up 40 seconds (healthy)"}, label: "1.3.0"}, true, "runs 1.3.0, not 1.4.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.host
			f := &fakeProc{answer: h.answer}
			r, _ := newRunner(f, tt.healthy)
			res := r.Deploy(context.Background(), target("staging"))
			if res.Status != pipeline.DeployFailed || !strings.Contains(res.Error, tt.want) {
				t.Errorf("result %+v, want failed with %q", res, tt.want)
			}
		})
	}
}

func TestRunner_WithoutHealthCheckUpIsEnough(t *testing.T) {
	h := &host{statuses: []string{"Up 3 seconds"}, label: "1.4.0"}
	r, _ := newRunner(&fakeProc{answer: h.answer}, false)
	if res := r.Deploy(context.Background(), target("staging")); res.Status != pipeline.DeployDeployed {
		t.Errorf("result %+v", res)
	}
}

type mapSource map[string]Data

func (m mapSource) Read(_ context.Context, env string) (Data, error) {
	d, ok := m[env]
	if !ok {
		return nil, errors.New(env + ": not found")
	}
	return d, nil
}

func deployConfig(selection string, envs ...pipelineconfig.Environment) *pipelineconfig.Deploy {
	return &pipelineconfig.Deploy{Selection: selection, Environments: envs, SSH: pipelineconfig.SSH{Auth: "password"}}
}

func TestPlan(t *testing.T) {
	prod, staging, dev := dataFor("blue"), dataFor("rc"), dataFor("alpha")
	prod["DOCKER_IMAGE_VERSION"], staging["DOCKER_IMAGE_VERSION"], dev["DOCKER_IMAGE_VERSION"] = "latest", "beta", "alpha"
	staging["DOCKER_DEPLOY_STRATEGY"] = "blue-green"
	delete(prod, "DOCKER_SSH_HOST") // not deployed by an rc: no need for its host
	src := mapSource{"production": prod, "staging": staging, "development": dev}
	cfg := deployConfig("target", pipelineconfig.Environment{Name: "production"}, pipelineconfig.Environment{Name: "staging"}, pipelineconfig.Environment{Name: "development"})
	plan, err := Plan(context.Background(), src, cfg, rel("1.4.0-rc.1", "release/1.4"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range plan {
		got = append(got, p.Result.Environment+":"+p.Result.Status)
	}
	if !slices.Equal(got, []string{"production:skipped", "staging:would-deploy", "development:skipped"}) || plan[1].Result.Strategy != "blue-green" || plan[1].Result.Container != "api-rc" {
		t.Errorf("plan %q %+v", got, plan)
	}

	delete(staging, "DOCKER_SSH_HOST")
	if _, err := Plan(context.Background(), src, cfg, rel("1.4.0-rc.1", "release/1.4")); err == nil || !strings.Contains(err.Error(), "staging: missing DOCKER_SSH_HOST") {
		t.Errorf("deployed environment without host: %v", err)
	}
	staging["DOCKER_SSH_HOST"], prod["DOCKER_SSH_HOST"] = "10.0.0.2", "10.0.0.3"
	delete(dev, "DOCKER_IMAGE_VERSION")
	if _, err := Plan(context.Background(), src, cfg, rel("1.4.0", "main")); err == nil || !strings.Contains(err.Error(), "development: DOCKER_IMAGE_VERSION is not set") {
		t.Errorf("no image version: %v", err)
	}
	if _, err := Plan(context.Background(), src, deployConfig("target", pipelineconfig.Environment{Name: "qa"}), rel("1.4.0", "main")); err == nil || !strings.Contains(err.Error(), "qa: not found") {
		t.Errorf("unreadable environment: %v", err)
	}
}

func TestRun_StopsAtTheFirstFailure(t *testing.T) {
	h := &host{statuses: []string{"Up (healthy)"}, label: "1.4.0"}
	f := &fakeProc{answer: func(c proc.Command) (proc.Result, error) {
		if strings.Contains(remoteOf(c), "api-b") && strings.HasPrefix(remoteOf(c), "cd ") {
			return proc.Result{}, errors.New("exit status 1")
		}
		return h.answer(c)
	}}
	r, _ := newRunner(f, true)
	plan := []Planned{
		{Target: target("a"), Result: pipeline.EnvironmentDeploy{Environment: "a", Status: pipeline.DeployWouldDeploy}, Deploy: true},
		{Result: pipeline.EnvironmentDeploy{Environment: "skip", Status: pipeline.DeploySkipped, Reason: "x"}},
		{Target: target("b"), Result: pipeline.EnvironmentDeploy{Environment: "b", Status: pipeline.DeployWouldDeploy}, Deploy: true},
		{Target: target("c"), Result: pipeline.EnvironmentDeploy{Environment: "c", Status: pipeline.DeployWouldDeploy}, Deploy: true},
	}
	envs, failed := r.Run(context.Background(), plan)
	var got []string
	for _, e := range envs {
		got = append(got, e.Environment+":"+e.Status)
	}
	if !slices.Equal(got, []string{"a:deployed", "skip:skipped", "b:failed", "c:not-run"}) || failed != "b" {
		t.Errorf("run %q failed %q", got, failed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	envs, _ = r.Run(ctx, plan)
	if envs[0].Status != pipeline.DeployNotRun || envs[3].Status != pipeline.DeployNotRun {
		t.Errorf("interrupted: %+v", envs)
	}
}
