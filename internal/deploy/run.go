package deploy

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
)

// Planned is one environment of the deploy: its target and whether the
// release deploys it (Result holds would-deploy or skipped and the reason).
type Planned struct {
	Target Target
	Result pipeline.EnvironmentDeploy
	Deploy bool
}

// Plan reads every configured environment (in order) from src and selects
// the ones rel deploys. A source that cannot be read, a missing
// DOCKER_IMAGE_VERSION with selection target, or a deployed environment
// missing a required key is an error naming the environment: nothing is
// deployed then.
func Plan(ctx context.Context, src Source, cfg *pipelineconfig.Deploy, rel pipeline.ReleaseOutputs) ([]Planned, error) {
	var plan []Planned
	for _, env := range cfg.Environments {
		data, err := src.Read(ctx, env.Name)
		if err != nil {
			return nil, err
		}
		t, terr := NewTarget(env.Name, data, cfg.SSH.Auth)
		d, err := Select(t, env, cfg.Selection, rel)
		if err != nil {
			return nil, err
		}
		if d.Deploy && terr != nil {
			return nil, terr
		}
		p := Planned{Target: t, Deploy: d.Deploy, Result: pipeline.EnvironmentDeploy{Environment: env.Name, Container: t.Container, Host: t.Host,
			Strategy: t.Strategy, ImageVersion: t.ImageVersion, Status: pipeline.DeploySkipped, Reason: d.Reason}}
		if d.Deploy {
			p.Result.Status = pipeline.DeployWouldDeploy
		}
		plan = append(plan, p)
	}
	return plan, nil
}

// Runner deploys targets and verifies them.
type Runner struct {
	SSH     SSH
	Verify  pipelineconfig.Verify
	Release pipeline.ReleaseOutputs
	// Output receives the compose script's output and one line per check;
	// nil discards them.
	Output io.Writer
	// Sleep and Now are the clock of the health wait; nil means the real
	// ones. PollInterval is the wait between checks (default 5s).
	Sleep        func(context.Context, time.Duration) error
	Now          func() time.Time
	PollInterval time.Duration
	// UpdateTimeout bounds the compose script's pull and up (default
	// pipelineconfig.DefaultUpdateTimeout); every health check and the
	// label read are bounded by the verify timeout.
	UpdateTimeout time.Duration
}

// call runs one remote command bounded by limit; a call cut off by it
// reports "timed out after <limit>".
func (r Runner) call(ctx context.Context, t Target, limit time.Duration, remote string, stream io.Writer) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	out, err := r.SSH.Run(cctx, t, remote, stream)
	if err != nil && cctx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		return out, fmt.Errorf("%s@%s: timed out after %s", t.User, t.Host, limit)
	}
	return out, err
}

func (r Runner) updateTimeout() time.Duration {
	if r.UpdateTimeout > 0 {
		return r.UpdateTimeout
	}
	d, _ := time.ParseDuration(pipelineconfig.DefaultUpdateTimeout)
	return d
}

// Run deploys the planned environments in order, one at a time, and stops
// at the first failure as update-deploy does (no rollback): the
// environments after it are not-run; skipped ones stay skipped. It returns
// every environment's result and the failed environment ("" when none).
// An interruption leaves the rest not-run.
func (r Runner) Run(ctx context.Context, plan []Planned) ([]pipeline.EnvironmentDeploy, string) {
	out := make([]pipeline.EnvironmentDeploy, len(plan))
	failed, stop := "", false
	for i, p := range plan {
		res := p.Result
		if p.Deploy {
			switch {
			case stop || ctx.Err() != nil:
				res.Status = pipeline.DeployNotRun
			default:
				res = r.Deploy(ctx, p.Target)
				if res.Status != pipeline.DeployDeployed {
					failed, stop = p.Target.Environment, true
				}
			}
		}
		out[i] = res
	}
	return out, failed
}

// Deploy updates t (compose script pull, then up) and verifies it: healthy
// (or at least Up without the health check) within the verify timeout, and
// its version label equal to the released version (a leading v ignored).
func (r Runner) Deploy(ctx context.Context, t Target) pipeline.EnvironmentDeploy {
	if err := r.Update(ctx, t); err != nil {
		res := newResult(t)
		res.Status, res.Error = pipeline.DeployFailed, err.Error()
		r.printf("%s: failed: %s\n", t.Environment, res.Error)
		return res
	}
	return r.Check(ctx, t)
}

func newResult(t Target) pipeline.EnvironmentDeploy {
	return pipeline.EnvironmentDeploy{Environment: t.Environment, Container: t.Container, Host: t.Host, Strategy: t.Strategy, ImageVersion: t.ImageVersion}
}

// Update runs the host's compose script for t's container: pull, then up,
// bounded by the update timeout.
func (r Runner) Update(ctx context.Context, t Target) error {
	c, script := shQuote(t.Container), shQuote(t.ComposeScript)
	update := fmt.Sprintf("cd %s && %s pull --container %s && %s up --container %s", shQuote(t.ComposePath), script, c, script, c)
	r.printf("%s: updating %s on %s\n", t.Environment, t.Container, t.Host)
	if _, err := r.call(ctx, t, r.updateTimeout(), update, r.Output); err != nil {
		return fmt.Errorf("updating %s: %v", t.Container, err)
	}
	return nil
}

// Check verifies t after an update: healthy (or at least Up without the
// health check) within the verify timeout, and its version label equal to
// the released version (a leading v ignored).
func (r Runner) Check(ctx context.Context, t Target) pipeline.EnvironmentDeploy {
	res := newResult(t)
	fail := func(format string, args ...any) pipeline.EnvironmentDeploy {
		res.Status, res.Error = pipeline.DeployFailed, fmt.Sprintf(format, args...)
		r.printf("%s: failed: %s\n", t.Environment, res.Error)
		return res
	}
	c := shQuote(t.Container)
	health, err := r.waitHealthy(ctx, t)
	res.Health = health
	if err != nil {
		return fail("%v", err)
	}
	inspect := fmt.Sprintf("docker inspect --format %s %s", shQuote(`{{index .Config.Labels "`+r.Verify.VersionLabel+`"}}`), c)
	label, err := r.call(ctx, t, r.Verify.TimeoutDuration(), inspect, nil)
	if err != nil {
		return fail("reading the version label of %s: %v", t.Container, err)
	}
	res.Running = strings.TrimSpace(label)
	if want := strings.TrimPrefix(r.Release.Version, "v"); strings.TrimPrefix(res.Running, "v") != want {
		return fail("%s runs %s, not %s (label %s)", t.Container, orNone(res.Running), r.Release.Version, r.Verify.VersionLabel)
	}
	res.Status = pipeline.DeployDeployed
	r.printf("%s: %s %s, version %s\n", t.Environment, t.Container, health, res.Running)
	return res
}

// waitHealthy polls docker ps until the container is (healthy) (or Up
// without the health check), failing at once when it is unhealthy or not
// running, and after the verify timeout otherwise.
func (r Runner) waitHealthy(ctx context.Context, t Target) (string, error) {
	ps := fmt.Sprintf("docker ps --all --filter %s --format %s", shQuote("name=^"+t.Container+"$"), shQuote("{{.Status}}"))
	now, sleep := r.Now, r.Sleep
	if now == nil {
		now = time.Now
	}
	if sleep == nil {
		sleep = func(ctx context.Context, d time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				return nil
			}
		}
	}
	interval := r.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	timeout := r.Verify.TimeoutDuration()
	deadline := now().Add(timeout)
	for {
		out, err := r.call(ctx, t, timeout, ps, nil)
		if err != nil {
			return "", fmt.Errorf("checking %s: %v", t.Container, err)
		}
		status := lastLine(strings.TrimSpace(out))
		up := strings.HasPrefix(status, "Up")
		switch {
		case status == "":
			return status, fmt.Errorf("no container %s on %s after the update", t.Container, t.Host)
		case strings.Contains(status, "(unhealthy)"):
			return status, fmt.Errorf("%s is unhealthy: %s", t.Container, status)
		case up && (!r.Verify.CheckHealthy() || strings.Contains(status, "(healthy)")):
			return status, nil
		case !up && !strings.HasPrefix(status, "Restarting"):
			return status, fmt.Errorf("%s is not running: %s", t.Container, status)
		}
		if !now().Before(deadline) {
			return status, fmt.Errorf("%s not healthy after %s: %s", t.Container, timeout, status)
		}
		if err := sleep(ctx, interval); err != nil {
			return status, err
		}
	}
}

func (r Runner) printf(format string, args ...any) {
	if r.Output != nil {
		fmt.Fprintf(r.Output, format, args...)
	}
}

func orNone(s string) string {
	if s == "" {
		return "no version label"
	}
	return s
}
