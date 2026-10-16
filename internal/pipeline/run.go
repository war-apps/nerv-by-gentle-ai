package pipeline

import "path/filepath"

// StepNotRun is the status of a step nerv pipeline run did not start
// because an earlier one did not pass.
const StepNotRun = "not-run"

// StepOutcome is one step of nerv pipeline run.
type StepOutcome struct {
	Step string `json:"step"`
	// Status is the step's own document status (passed, refused, failed,
	// interrupted), or not-run.
	Status string `json:"status"`
	// Exit is the step's exit code (0 when not run).
	Exit  int    `json:"exit"`
	Error string `json:"error,omitempty"`
}

// RunResult is nerv pipeline run's document (run.json, --json).
type RunResult struct {
	Step   string        `json:"step"`
	Status string        `json:"status"`
	DryRun bool          `json:"dry_run"`
	Steps  []StepOutcome `json:"steps"`
	// Error is the failing step's error, prefixed with its name.
	Error string `json:"error,omitempty"`
}

// RunResultFile is nerv pipeline run's summary.
const RunResultFile = "run.json"

// RunResultPath is <repo>/.nerv/artifacts/pipeline/run.json.
func RunResultPath(repo string) string {
	return filepath.Join(repo, ".nerv", "artifacts", "pipeline", RunResultFile)
}

// WriteRunResult writes run.json as WriteReleaseOutputs writes
// release.json.
func WriteRunResult(repo string, r RunResult) (string, error) {
	return writeArtifactJSON(repo, RunResultPath(repo), r)
}
