// Package deploy is the release-ssh-compose provider of nerv deploy: dim's
// /update-deploy for one environment. It reads the environment's deploy data
// (environment variables or Infisical, update-deploy's layout), decides
// whether the release deploys it, updates its container over SSH with the
// host's compose script and verifies that the container is healthy and runs
// the released version.
package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// ErrIncompleteData is wrapped when an environment's deploy data lacks a
// key the deploy needs or holds an invalid one (an unsafe user or host): a
// configuration problem, nothing is deployed.
var ErrIncompleteData = errors.New("incomplete deploy data")

// Data is an environment's deploy keys (HostKeys and AppKeys) as a source
// gives them.
type Data map[string]string

// Source reads an environment's deploy data.
type Source interface {
	Read(ctx context.Context, environment string) (Data, error)
}

// EnvSource reads NERV_DEPLOY_<ENVIRONMENT>_<KEY> (EnvVar).
type EnvSource struct {
	Getenv func(string) string
}

// Read returns the environment's keys that are set.
func (s EnvSource) Read(_ context.Context, environment string) (Data, error) {
	d := Data{}
	for _, key := range append(HostKeys(), AppKeys()...) {
		if v := s.Getenv(EnvVar(environment, key)); v != "" {
			d[key] = v
		}
	}
	return d, nil
}

// InfisicalSource reads update-deploy's single-folder layout with the
// infisical CLI and the user's `infisical login` session: the host keys at
// the environment's root, the application's keys under AppPath. Values are
// raw, as update-deploy reads them (no imports, no expansion, no personal
// overrides).
type InfisicalSource struct {
	Proc      proc.Runner
	ProjectID string
	AppPath   string
	// Domain is passed as --domain when set (otherwise the CLI session's).
	Domain string
	// Env is the CLI's environment (it needs HOME for the session).
	Env []string
}

// Read reads the root and the application folder of the environment.
func (s InfisicalSource) Read(ctx context.Context, environment string) (Data, error) {
	d := Data{}
	for _, part := range []struct {
		path string
		keys []string
	}{{"/", HostKeys()}, {s.AppPath, AppKeys()}} {
		values, err := s.export(ctx, environment, part.path)
		if err != nil {
			return nil, err
		}
		for _, key := range part.keys {
			if v := values[key]; v != "" {
				d[key] = v
			}
		}
	}
	return d, nil
}

func (s InfisicalSource) export(ctx context.Context, environment, path string) (map[string]string, error) {
	args := []string{"export", "--silent", "--env", environment, "--path", path, "--projectId", s.ProjectID, "--format", "json",
		"--include-imports=false", "--expand=false", "--secret-overriding=false"}
	if s.Domain != "" {
		args = append(args, "--domain", s.Domain)
	}
	res, err := s.Proc.Run(ctx, proc.Command{Name: "infisical", Args: args, Env: s.Env})
	if err != nil {
		// Never the output: it holds the secrets.
		detail := strings.TrimSpace(string(res.Stderr))
		if detail != "" {
			detail = ": " + lastLine(detail)
		}
		return nil, fmt.Errorf("reading Infisical %s:%s (infisical export: %v%s; check `infisical login`)", environment, path, err, detail)
	}
	var secrets []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(res.Stdout, &secrets); err != nil {
		return nil, fmt.Errorf("reading Infisical %s:%s: the CLI did not print a JSON list of secrets", environment, path)
	}
	values := map[string]string{}
	for _, sc := range secrets {
		values[sc.Key] = sc.Value
	}
	return values, nil
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Target is one environment's container, built from its Data.
type Target struct {
	Environment   string
	Host          string
	User          string
	Password      string // never printed or recorded
	ComposePath   string
	ComposeScript string
	// Container is <DOCKER_CONTAINER_NAME>-<DOCKER_CONTAINER_VARIANT>.
	Container    string
	Variant      string
	Image        string
	ImageVersion string
	Strategy     string
	// Enabled is false for DOCKER_DEPLOY_ENABLED false, 0, no or off.
	Enabled bool
}

// String describes the target without its password.
func (t Target) String() string {
	return fmt.Sprintf("%s %s@%s %s", t.Environment, t.User, t.Host, t.Container)
}

// NewTarget builds the environment's target; a missing required key is an
// error naming every missing one (DOCKER_SSH_PASS only with auth password).
func NewTarget(environment string, d Data, auth string) (Target, error) {
	required := []string{"DOCKER_SSH_HOST", "DOCKER_SSH_USER", "DOCKER_COMPOSE_PATH", "DOCKER_COMPOSE_SCRIPT", "DOCKER_CONTAINER_NAME", "DOCKER_CONTAINER_VARIANT"}
	if auth == "password" {
		required = append(required, "DOCKER_SSH_PASS")
	}
	var missing []string
	for _, k := range required {
		if d[k] == "" {
			missing = append(missing, k)
		}
	}
	t := Target{Environment: environment, Host: d["DOCKER_SSH_HOST"], User: d["DOCKER_SSH_USER"], Password: d["DOCKER_SSH_PASS"],
		ComposePath: d["DOCKER_COMPOSE_PATH"], ComposeScript: d["DOCKER_COMPOSE_SCRIPT"], Variant: d["DOCKER_CONTAINER_VARIANT"],
		Image: d["DOCKER_IMAGE"], ImageVersion: d["DOCKER_IMAGE_VERSION"], Strategy: d["DOCKER_DEPLOY_STRATEGY"],
		Enabled: !slices.Contains([]string{"false", "0", "no", "off"}, strings.ToLower(d["DOCKER_DEPLOY_ENABLED"]))}
	if d["DOCKER_CONTAINER_NAME"] != "" && t.Variant != "" {
		t.Container = d["DOCKER_CONTAINER_NAME"] + "-" + t.Variant
	}
	if len(missing) > 0 {
		return t, fmt.Errorf("%s: missing %s: %w", environment, strings.Join(missing, ", "), ErrIncompleteData)
	}
	if err := checkDestination(t.User, t.Host); err != nil {
		return t, fmt.Errorf("%s: %v: %w", environment, err, ErrIncompleteData)
	}
	return t, nil
}
