package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Environment deploy statuses.
const (
	DeployDeployed    = "deployed"
	DeploySkipped     = "skipped"
	DeployFailed      = "failed"
	DeployNotRun      = "not-run"
	DeployWouldDeploy = "would-deploy"
)

// EnvironmentDeploy is one environment of the deploy step.
type EnvironmentDeploy struct {
	Environment string `json:"environment"`
	Container   string `json:"container,omitempty"`
	Host        string `json:"host,omitempty"`
	Strategy    string `json:"strategy,omitempty"`
	// ImageVersion is the environment's DOCKER_IMAGE_VERSION.
	ImageVersion string `json:"image_version,omitempty"`
	// Status: deployed, skipped, failed, not-run; in a dry run
	// would-deploy or skipped.
	Status string `json:"status"`
	// Reason says why the environment is skipped.
	Reason string `json:"reason,omitempty"`
	// Health is the container's last docker ps status.
	Health string `json:"health,omitempty"`
	// Running is the deployed container's version label.
	Running string `json:"running,omitempty"`
	Error   string `json:"error,omitempty"`
}

// DeployResult is the deploy step's document (deploy.json, --json).
type DeployResult struct {
	Step         string              `json:"step"`
	Status       string              `json:"status"`
	DryRun       bool                `json:"dry_run"`
	Version      string              `json:"version,omitempty"`
	GitTag       string              `json:"git_tag,omitempty"`
	Branch       string              `json:"branch,omitempty"`
	Source       string              `json:"source,omitempty"`
	Selection    string              `json:"selection,omitempty"`
	Environments []EnvironmentDeploy `json:"environments"`
	Error        string              `json:"error,omitempty"`
	Action       string              `json:"action,omitempty"`
}

// DeployResultFile is the deploy step's result.
const DeployResultFile = "deploy.json"

// DeployResultPath is <repo>/.nerv/artifacts/pipeline/deploy.json.
func DeployResultPath(repo string) string {
	return filepath.Join(repo, ".nerv", "artifacts", "pipeline", DeployResultFile)
}

// WriteDeployResult writes deploy.json as WriteReleaseOutputs writes
// release.json.
func WriteDeployResult(repo string, r DeployResult) (string, error) {
	return writeArtifactJSON(repo, DeployResultPath(repo), r)
}

// ErrNoDeployResult is wrapped by ReadDeployResult when there is no
// deploy.json.
var ErrNoDeployResult = errors.New("no deploy result")

// ReadDeployResult reads deploy.json, after the artifacts' symlink checks.
func ReadDeployResult(repo string) (DeployResult, error) {
	var r DeployResult
	err := readArtifactJSON(repo, DeployResultPath(repo), &r)
	if errors.Is(err, os.ErrNotExist) {
		return r, fmt.Errorf("%s: %w", DeployResultPath(repo), ErrNoDeployResult)
	}
	return r, err
}

// CheckDeployEvidence requires, for the release in rel, a passing
// docker.json of its version, tag and released commit (as
// CheckSyncEvidence does for docker; the deploy updates containers to the
// pushed image). A missing, failed, stale or mismatched record wraps
// ErrMissingEvidence, prefixed "docker: ".
func CheckDeployEvidence(repo string, rel ReleaseOutputs) error {
	return checkDockerRecord(repo, rel)
}
