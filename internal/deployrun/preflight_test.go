package deployrun

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// lookPathWithout is exec.LookPath with the named tools missing.
func lookPathWithout(missing ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, m := range missing {
			if name == m {
				return "", exec.ErrNotFound
			}
		}
		return exec.LookPath(name)
	}
}

// refusedBeforeAnyWrite checks that the deploy wrote nothing: no clone, no
// push, no provider run.
func refusedBeforeAnyWrite(t *testing.T, e *world, rep Report, before string) {
	t.Helper()
	if e.originRefs() != before {
		t.Errorf("origin changed")
	}
	if got := statuses(rep)["clone"]; got != StatusNotRun {
		t.Errorf("clone step %q, want not-run: the preflight comes first", got)
	}
	if log := e.log(); len(log) != 1 || log[0] != "" {
		t.Errorf("providers ran: %v", log)
	}
}

// L15: every target's tools and config are checked before any git write:
// sshpass for password authentication, infisical for source infisical, and
// the provider's own config checks.
func TestRun_PreflightsEveryTargetBeforeAnyGitWrite(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]string
		missing []string
		want    string
	}{
		{"password auth needs sshpass", map[string]string{"environment": "staging"}, []string{"sshpass"}, "sshpass is not on PATH"},
		{"source infisical needs the CLI", map[string]string{"environment": "staging", "source": "infisical", "infisical_project_id": "p", "infisical_app_path": "/app"},
			[]string{"infisical"}, "infisical is not on PATH"},
		{"key auth needs no sshpass but a key file", map[string]string{"environment": "staging", "ssh_auth": "key"}, []string{"sshpass"}, "config.ssh_key_file is required by ssh_auth key"},
		{"source infisical needs its project", map[string]string{"environment": "staging", "source": "infisical"}, nil, "config.infisical_project_id and config.infisical_app_path are required"},
		{"the environment is required", map[string]string{}, nil, "config.environment is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.cfg.Targets[0].Config = tt.config
			before := e.originRefs()
			opts := e.options()
			opts.LookPath = lookPathWithout(tt.missing...)
			rep, err := e.run(opts)
			if !refusal.Is(err) || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "staging") {
				t.Fatalf("Run: %v, want a refusal naming the target and %q", err, tt.want)
			}
			refusedBeforeAnyWrite(t, e, rep, before)
		})
	}
}

// L15: --rebase says that the tag points at code nerv ship never built.
func TestRun_RebaseWarnsThatTheTaggedCodeWasNeverBuilt(t *testing.T) {
	e := newEnv(t)
	e.pushOther("other.txt", "x\n")
	var out bytes.Buffer
	opts := e.options()
	opts.Rebase, opts.Output = true, &out
	if _, err := e.run(opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "warning: --rebase") || !strings.Contains(out.String(), "never built") || !strings.Contains(out.String(), short(e.base)) {
		t.Errorf("no rebase warning:\n%s", out.String())
	}
}

// L15: a failed push names git push, not its -c option.
func TestRun_PushFailureNamesGitPush(t *testing.T) {
	e := newEnv(t)
	writeFile(t, filepath.Join(e.origin, "hooks", "pre-receive"), "#!/bin/sh\necho 'rejected by policy' >&2\nexit 1\n", 0o755)
	_, err := e.run(e.options())
	if err == nil || !strings.Contains(err.Error(), "git push:") || strings.Contains(err.Error(), "git -c") {
		t.Fatalf("Run: %v, want an error naming git push", err)
	}
}

// L15: gh's expected "release not found" while checking for the release is
// not shown.
func TestRun_SilencesTheExpectedReleaseNotFound(t *testing.T) {
	e := newEnv(t)
	var out bytes.Buffer
	opts := e.options()
	opts.Output = &out
	if _, err := e.run(opts); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "release not found") {
		t.Errorf("gh's expected stderr is shown:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(e.fake, "release-"+tag)); err != nil {
		t.Errorf("the release was not created: %v", err)
	}
}
