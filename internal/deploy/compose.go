package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// ComposeDeps are the effects of the deploy-ssh-compose operations.
type ComposeDeps struct {
	// Proc runs infisical, ssh and sshpass.
	Proc proc.Runner
	// Getenv reads the NERV_DEPLOY_* data of source env.
	Getenv func(string) string
	// Home expands a leading ~/ in the key and known_hosts files.
	Home string
	// Env is the environment of infisical and ssh; nil means os.Environ.
	Env []string
	// Output receives progress (stderr); the response goes to stdout.
	Output io.Writer
	// Sleep waits between health checks; nil means a real wait.
	Sleep func(context.Context, time.Duration) error
}

// composeRequest is the part of the deploy-provider request the
// operations read.
type composeRequest struct {
	Operation string            `json:"operation"`
	Target    string            `json:"target"`
	Config    map[string]string `json:"config"`
	Artifact  struct {
		Version string `json:"version"`
		Branch  string `json:"branch"`
		Tag     string `json:"tag"`
	} `json:"artifact"`
}

type composeResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// RunComposeOp is `nerv deploy-ssh-compose <op>`, the deploy and verify
// operations of the deploy-ssh-compose provider: it reads the request on
// stdin, the environment's deploy data from its source, and then deploy
// runs the host's compose script (pull, up) and verify waits for the
// container's health and checks its version label. It prints one JSON
// response and returns 0 (ok or skipped) or 1 (failed). An environment
// whose DOCKER_DEPLOY_ENABLED is off, or whose DOCKER_IMAGE_VERSION points
// at other releases, is skipped.
func RunComposeOp(ctx context.Context, op string, stdin io.Reader, stdout io.Writer, deps ComposeDeps) int {
	status, msg := composeOp(ctx, op, stdin, deps)
	data, _ := json.Marshal(composeResponse{Status: status, Message: msg})
	fmt.Fprintf(stdout, "%s\n", data)
	if status == "failed" {
		return 1
	}
	return 0
}

func composeOp(ctx context.Context, op string, stdin io.Reader, deps ComposeDeps) (string, string) {
	fail := func(format string, args ...any) (string, string) { return "failed", fmt.Sprintf(format, args...) }
	if op != "deploy" && op != "verify" {
		return fail("unknown operation %q (want deploy or verify)", op)
	}
	var req composeRequest
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		return fail("reading the request on stdin: %v", err)
	}
	if req.Operation != op {
		return fail("the request is for operation %q, not %s", req.Operation, op)
	}
	cfg, err := composeConfig(req.Config, deps.Home)
	if err != nil {
		return fail("%v", err)
	}
	env := deps.Env
	if env == nil {
		env = os.Environ()
	}
	var src Source = EnvSource{Getenv: deps.Getenv}
	if cfg.source == "infisical" {
		src = InfisicalSource{Proc: deps.Proc, ProjectID: cfg.infisical.ProjectID, AppPath: cfg.infisical.AppPath, Domain: cfg.infisical.Domain, Env: env}
	}
	data, err := src.Read(ctx, cfg.environment)
	if err != nil {
		return fail("%v", err)
	}
	t, err := NewTarget(cfg.environment, data, cfg.ssh.Auth)
	if !t.Enabled {
		return "skipped", fmt.Sprintf("%s: DOCKER_DEPLOY_ENABLED is off", cfg.environment)
	}
	rel := pipeline.ReleaseOutputs{Version: req.Artifact.Version, GitTag: req.Artifact.Tag, Branch: req.Artifact.Branch}
	if err != nil {
		return fail("%v", err)
	}
	// The default target selection, as nerv pipeline deploy applies it: an
	// environment that names no DOCKER_IMAGE_VERSION is refused, so a
	// prerelease never reaches an environment by default.
	d, serr := Select(t, pipelineconfig.Environment{Name: cfg.environment}, "target", rel)
	if serr != nil {
		return fail("%v", serr)
	}
	if !d.Deploy {
		return "skipped", fmt.Sprintf("%s: %s", cfg.environment, d.Reason)
	}
	ssh := SSH{Proc: deps.Proc, Auth: cfg.ssh.Auth, KeyFile: cfg.ssh.KeyFile, KnownHosts: cfg.ssh.KnownHosts, Port: cfg.ssh.Port,
		ConnectTimeout: cfg.ssh.ConnectTimeoutDuration(), Env: env}
	r := Runner{SSH: ssh, Verify: cfg.verify, Release: rel, Output: deps.Output, Sleep: deps.Sleep, UpdateTimeout: cfg.updateTimeout}
	if op == "deploy" {
		if err := r.Update(ctx, t); err != nil {
			return fail("%s: %v", cfg.environment, err)
		}
		return "ok", fmt.Sprintf("%s: %s updated on %s", cfg.environment, t.Container, t.Host)
	}
	res := r.Check(ctx, t)
	if res.Status != pipeline.DeployDeployed {
		return fail("%s: %s", cfg.environment, res.Error)
	}
	return "ok", fmt.Sprintf("%s: %s %s, version %s", cfg.environment, t.Container, res.Health, res.Running)
}

type composeSettings struct {
	environment, source string
	infisical           pipelineconfig.Infisical
	ssh                 pipelineconfig.SSH
	verify              pipelineconfig.Verify
	updateTimeout       time.Duration
}

// composeConfig reads and checks the provider's config keys (their
// defaults are already applied by nerv deploy).
func composeConfig(c map[string]string, home string) (composeSettings, error) {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	s := composeSettings{environment: c["environment"], source: c["source"]}
	if s.environment == "" {
		add("config.environment is required")
	}
	if !slices.Contains(pipelineconfig.DeploySources, s.source) {
		add("config.source %q must be one of %s", s.source, strings.Join(pipelineconfig.DeploySources, ", "))
	}
	s.infisical = pipelineconfig.Infisical{ProjectID: c["infisical_project_id"], AppPath: c["infisical_app_path"], Domain: c["infisical_domain"]}
	if s.source == "infisical" && (s.infisical.ProjectID == "" || s.infisical.AppPath == "") {
		add("config.infisical_project_id and config.infisical_app_path are required by source infisical")
	}
	s.ssh = pipelineconfig.SSH{Auth: c["ssh_auth"], KeyFile: expandHome(c["ssh_key_file"], home), KnownHosts: expandHome(c["ssh_known_hosts"], home), ConnectTimeout: c["ssh_connect_timeout"]}
	if !slices.Contains(pipelineconfig.SSHAuths, s.ssh.Auth) {
		add("config.ssh_auth %q must be one of %s", s.ssh.Auth, strings.Join(pipelineconfig.SSHAuths, ", "))
	}
	if s.ssh.Auth == "key" && s.ssh.KeyFile == "" {
		add("config.ssh_key_file is required by ssh_auth key")
	}
	if s.ssh.KnownHosts == "" {
		add("config.ssh_known_hosts is required")
	}
	port, err := strconv.Atoi(c["ssh_port"])
	if err != nil || port < 1 || port > 65535 {
		add("config.ssh_port %q is not a port", c["ssh_port"])
	}
	s.ssh.Port = port
	healthy, err := strconv.ParseBool(c["verify_healthy"])
	if err != nil {
		add("config.verify_healthy %q is not a bool", c["verify_healthy"])
	}
	s.verify = pipelineconfig.Verify{Healthy: &healthy, VersionLabel: c["version_label"], Timeout: c["verify_timeout"]}
	if s.verify.VersionLabel == "" {
		add("config.version_label is required")
	}
	for _, k := range []string{"ssh_connect_timeout", "update_timeout", "verify_timeout"} {
		if d, err := time.ParseDuration(c[k]); err != nil || d <= 0 {
			add("config.%s %q is not a positive Go duration", k, c[k])
		} else if k == "update_timeout" {
			s.updateTimeout = d
		}
	}
	if len(problems) > 0 {
		return s, fmt.Errorf("deploy-ssh-compose: %s", strings.Join(problems, "; "))
	}
	return s, nil
}

// expandHome expands a leading ~/ to home.
func expandHome(p, home string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok && home != "" {
		return filepath.Join(home, filepath.FromSlash(rest))
	}
	return p
}
