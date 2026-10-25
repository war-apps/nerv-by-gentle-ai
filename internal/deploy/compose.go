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

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
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

// composeRequest is the part of the provider request (the common
// envelope, extension.Request, with deploy's payload) the operations read.
type composeRequest struct {
	Operation string            `json:"operation"`
	Config    map[string]string `json:"config"`
	Payload   struct {
		Target   string `json:"target"`
		Artifact struct {
			Version string `json:"version"`
			Branch  string `json:"branch"`
			Tag     string `json:"tag"`
		} `json:"artifact"`
	} `json:"payload"`
}

// The operations of the deploy-ssh-compose provider.
const (
	opPreflight = "preflight"
	opDeploy    = "deploy"
	opVerify    = "verify"
)

// RunComposeOp is `nerv provider deploy-ssh-compose <op>`, the native
// operations of the deploy-ssh-compose provider: it reads the request on
// stdin and prints one JSON response (the common envelope). preflight
// checks the config before nerv deploy writes anything and answers, in
// data.requires, the tools it needs on PATH besides the provider's
// requires: sshpass for password authentication, infisical for source
// infisical. deploy and verify read the environment's deploy data from its
// source, then deploy runs the host's compose script (pull, up) and verify
// waits for the container's health and checks its version label. It
// returns 0 (ok or skipped) or 1 (failed). An environment whose
// DOCKER_DEPLOY_ENABLED is off, or whose DOCKER_IMAGE_VERSION points at
// other releases, is skipped.
func RunComposeOp(ctx context.Context, op string, stdin io.Reader, stdout io.Writer, deps ComposeDeps) int {
	resp := composeOp(ctx, op, stdin, deps)
	data, _ := json.Marshal(resp)
	fmt.Fprintf(stdout, "%s\n", data)
	if resp.Failure() {
		return 1
	}
	return 0
}

func composeOp(ctx context.Context, op string, stdin io.Reader, deps ComposeDeps) extension.Response {
	answer := func(status, format string, args ...any) extension.Response {
		return extension.Response{Status: status, Message: fmt.Sprintf(format, args...)}
	}
	fail := func(format string, args ...any) extension.Response {
		return answer(extension.StatusFailed, format, args...)
	}
	if op != opPreflight && op != opDeploy && op != opVerify {
		return fail("unknown operation %q (want preflight, deploy or verify)", op)
	}
	var req composeRequest
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		return fail("reading the request on stdin: %v", err)
	}
	if req.Operation != op {
		return fail("the request is for operation %q, not %s", req.Operation, op)
	}
	if op == opPreflight {
		// The ~/ of the key and known_hosts files is not expanded: the
		// preflight checks the config, it reads no file.
		cfg, err := composeConfig(req.Config, "")
		if err != nil {
			return fail("%v", err)
		}
		data, _ := json.Marshal(struct {
			Requires []string `json:"requires"`
		}{cfg.tools()})
		return extension.Response{Status: extension.StatusOK, Data: data}
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
		return answer(extension.StatusSkipped, "%s: DOCKER_DEPLOY_ENABLED is off", cfg.environment)
	}
	a := req.Payload.Artifact
	rel := Release{Version: a.Version, GitTag: a.Tag, Branch: a.Branch}
	if err != nil {
		return fail("%v", err)
	}
	// An environment that names no DOCKER_IMAGE_VERSION is refused, so a
	// prerelease never reaches an environment by default.
	d, serr := Select(t, rel)
	if serr != nil {
		return fail("%v", serr)
	}
	if !d.Deploy {
		return answer(extension.StatusSkipped, "%s: %s", cfg.environment, d.Reason)
	}
	ssh := SSH{Proc: deps.Proc, Auth: cfg.ssh.Auth, KeyFile: cfg.ssh.KeyFile, KnownHosts: cfg.ssh.KnownHosts, Port: cfg.ssh.Port,
		ConnectTimeout: cfg.ssh.ConnectTimeout, Env: env}
	r := Runner{SSH: ssh, Verify: cfg.verify, Release: rel, Output: deps.Output, Sleep: deps.Sleep, UpdateTimeout: cfg.updateTimeout}
	if op == opDeploy {
		if err := r.Update(ctx, t); err != nil {
			return fail("%s: %v", cfg.environment, err)
		}
		return answer(extension.StatusOK, "%s: %s updated on %s", cfg.environment, t.Container, t.Host)
	}
	res := r.Check(ctx, t)
	if res.Status != StatusDeployed {
		return fail("%s: %s", cfg.environment, res.Error)
	}
	return answer(extension.StatusOK, "%s: %s %s, version %s", cfg.environment, t.Container, res.Health, res.Running)
}

type composeSettings struct {
	environment, source string
	infisical           Infisical
	ssh                 SSHConfig
	verify              Verify
	updateTimeout       time.Duration
}

// tools are the programs the settings need besides the provider's
// requires (docker, ssh): sshpass for password authentication, the
// infisical CLI for source infisical.
func (s composeSettings) tools() []string {
	var tools []string
	if s.ssh.Auth == "password" {
		tools = append(tools, "sshpass")
	}
	if s.source == "infisical" {
		tools = append(tools, "infisical")
	}
	return tools
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
	if !slices.Contains(Sources, s.source) {
		add("config.source %q must be one of %s", s.source, strings.Join(Sources, ", "))
	}
	s.infisical = Infisical{ProjectID: c["infisical_project_id"], AppPath: c["infisical_app_path"], Domain: c["infisical_domain"]}
	if s.source == "infisical" && (s.infisical.ProjectID == "" || s.infisical.AppPath == "") {
		add("config.infisical_project_id and config.infisical_app_path are required by source infisical")
	}
	s.ssh = SSHConfig{Auth: c["ssh_auth"], KeyFile: expandHome(c["ssh_key_file"], home), KnownHosts: expandHome(c["ssh_known_hosts"], home)}
	if !slices.Contains(SSHAuths, s.ssh.Auth) {
		add("config.ssh_auth %q must be one of %s", s.ssh.Auth, strings.Join(SSHAuths, ", "))
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
	s.verify = Verify{Healthy: healthy, VersionLabel: c["version_label"]}
	if s.verify.VersionLabel == "" {
		add("config.version_label is required")
	}
	for _, k := range []string{"ssh_connect_timeout", "update_timeout", "verify_timeout"} {
		if d, err := time.ParseDuration(c[k]); err != nil || d <= 0 {
			add("config.%s %q is not a positive Go duration", k, c[k])
		} else {
			switch k {
			case "ssh_connect_timeout":
				s.ssh.ConnectTimeout = d
			case "update_timeout":
				s.updateTimeout = d
			case "verify_timeout":
				s.verify.Timeout = d
			}
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
