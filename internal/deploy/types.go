package deploy

import (
	"strings"
	"time"
)

// Release is the release a release-ssh-compose operation deploys: the
// artifact's version, tag and branch.
type Release struct {
	Version string
	GitTag  string
	Branch  string
}

// Environment deploy statuses.
const (
	StatusDeployed = "deployed"
	StatusFailed   = "failed"
)

// Result is one environment's deploy.
type Result struct {
	Environment string `json:"environment"`
	Container   string `json:"container,omitempty"`
	Host        string `json:"host,omitempty"`
	Strategy    string `json:"strategy,omitempty"`
	// ImageVersion is the environment's DOCKER_IMAGE_VERSION.
	ImageVersion string `json:"image_version,omitempty"`
	// Status: deployed or failed.
	Status string `json:"status"`
	// Health is the container's last docker ps status.
	Health string `json:"health,omitempty"`
	// Running is the deployed container's version label.
	Running string `json:"running,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Sources are the places the deploy data can come from.
var Sources = []string{"env", "infisical"}

// SSHAuths are the SSH authentication methods.
var SSHAuths = []string{"password", "key"}

// DefaultUpdateTimeout bounds the compose script's pull and up when no
// update timeout is given.
const DefaultUpdateTimeout = 15 * time.Minute

// EnvPrefix starts the variables of source env.
const EnvPrefix = "NERV_DEPLOY"

// HostKeys are the deploy keys of an environment's Docker host (the
// environment root in Infisical).
func HostKeys() []string {
	return []string{"DOCKER_SSH_HOST", "DOCKER_SSH_USER", "DOCKER_SSH_PASS", "DOCKER_COMPOSE_PATH", "DOCKER_COMPOSE_SCRIPT"}
}

// AppKeys are the deploy keys of the application in an environment (the app
// folder in Infisical).
func AppKeys() []string {
	return []string{
		"DOCKER_CONTAINER_NAME", "DOCKER_CONTAINER_VARIANT", "DOCKER_DEPLOY_ENABLED",
		"DOCKER_DEPLOY_STRATEGY", "DOCKER_IMAGE", "DOCKER_IMAGE_VERSION",
	}
}

// EnvVar is the variable holding a deploy key for an environment with
// source env: NERV_DEPLOY_<ENVIRONMENT>_<KEY>, the environment upper-cased
// with - turned into _ (production-blue, DOCKER_SSH_HOST ->
// NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_SSH_HOST).
func EnvVar(environment, key string) string {
	return EnvPrefix + "_" + strings.ToUpper(strings.ReplaceAll(environment, "-", "_")) + "_" + key
}

// Infisical locates the deploy data in Infisical (read through the user's
// `infisical login` CLI session).
type Infisical struct {
	ProjectID string
	AppPath   string
	// Domain is the Infisical URL; empty uses the CLI session's.
	Domain string
}

// SSHConfig configures the connection to the Docker hosts.
type SSHConfig struct {
	// Auth is password (DOCKER_SSH_PASS through sshpass) or key (KeyFile).
	Auth           string
	KeyFile        string
	KnownHosts     string
	Port           int
	ConnectTimeout time.Duration
}

// Verify configures the check after each deploy.
type Verify struct {
	// Healthy waits for the container's (healthy) status.
	Healthy bool
	// VersionLabel is the image label compared with the deployed version.
	VersionLabel string
	// Timeout bounds the wait.
	Timeout time.Duration
}
