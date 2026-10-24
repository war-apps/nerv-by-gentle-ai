package deploy

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// Runner deploys targets and verifies them.
type Runner struct {
	SSH     SSH
	Verify  Verify
	Release Release
	// Output receives the compose script's output and one line per check;
	// nil discards them.
	Output io.Writer
	// Sleep and Now are the clock of the health wait; nil means the real
	// ones. PollInterval is the wait between checks (default 5s).
	Sleep        func(context.Context, time.Duration) error
	Now          func() time.Time
	PollInterval time.Duration
	// UpdateTimeout bounds the compose script's pull and up (default
	// DefaultUpdateTimeout); every health check and the label read are
	// bounded by the verify timeout.
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
	return DefaultUpdateTimeout
}

// Deploy updates t (compose script pull, then up) and verifies it: healthy
// (or at least Up without the health check) within the verify timeout, and
// its version label equal to the released version (a leading v ignored).
func (r Runner) Deploy(ctx context.Context, t Target) Result {
	if err := r.Update(ctx, t); err != nil {
		res := newResult(t)
		res.Status, res.Error = StatusFailed, err.Error()
		r.printf("%s: failed: %s\n", t.Environment, res.Error)
		return res
	}
	return r.Check(ctx, t)
}

func newResult(t Target) Result {
	return Result{Environment: t.Environment, Container: t.Container, Host: t.Host, Strategy: t.Strategy, ImageVersion: t.ImageVersion}
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
func (r Runner) Check(ctx context.Context, t Target) Result {
	res := newResult(t)
	fail := func(format string, args ...any) Result {
		res.Status, res.Error = StatusFailed, fmt.Sprintf(format, args...)
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
	label, err := r.call(ctx, t, r.Verify.Timeout, inspect, nil)
	if err != nil {
		return fail("reading the version label of %s: %v", t.Container, err)
	}
	res.Running = strings.TrimSpace(label)
	if want := strings.TrimPrefix(r.Release.Version, "v"); strings.TrimPrefix(res.Running, "v") != want {
		return fail("%s runs %s, not %s (label %s)", t.Container, orNone(res.Running), r.Release.Version, r.Verify.VersionLabel)
	}
	res.Status = StatusDeployed
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
	timeout := r.Verify.Timeout
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
		case up && (!r.Verify.Healthy || strings.Contains(status, "(healthy)")):
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
