package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TestRecordFile is the test step's record of a passing run, for sync.
const TestRecordFile = "test.json"

// TestRecord is what a passing `nerv pipeline test` tested: the working
// tree's HEAD and whether the tree had changes outside .nerv/ (the tests
// ran on them). It is written only when the step passed; a failed run
// leaves an earlier record as it was.
type TestRecord struct {
	Step   string `json:"step"`
	Status string `json:"status"`
	// Commit is HEAD when the tests ran.
	Commit string `json:"commit"`
	// Dirty: the working tree had changes outside .nerv/ (Changes).
	Dirty   bool     `json:"dirty"`
	Changes []string `json:"changes,omitempty"`
	// Coverage is the coverage gate of the run: its mode, the thresholds
	// checked (and their source) and the measured values.
	Coverage *CoverageResult `json:"coverage,omitempty"`
}

// TestRecordPath is <repo>/.nerv/artifacts/pipeline/test.json.
func TestRecordPath(repo string) string {
	return filepath.Join(repo, ".nerv", "artifacts", "pipeline", TestRecordFile)
}

// WriteTestRecord writes test.json as WriteReleaseOutputs writes
// release.json.
func WriteTestRecord(repo string, r TestRecord) (string, error) {
	return writeArtifactJSON(repo, TestRecordPath(repo), r)
}

// ReadTestRecord reads test.json; a missing one wraps os.ErrNotExist.
func ReadTestRecord(repo string) (TestRecord, error) {
	var r TestRecord
	return r, readArtifactJSON(repo, TestRecordPath(repo), &r)
}

// ErrMissingEvidence is wrapped by CheckSyncEvidence and
// CheckDeployEvidence when an earlier step gives no passing, current record
// for the release: sync merges nothing, deploy deploys nothing.
var ErrMissingEvidence = errors.New("no passing record for this release")

// CheckSyncEvidence requires, for the release in rel, a passing record of
// every earlier step that sync depends on (the user's rule: if build, test
// or docker failed, sync nothing):
//   - test.json passed, with no uncommitted changes, at rel.BaseCommit (the
//     commit the release was made from);
//   - docker.json passed for rel's version, tag and released commit
//     (git_head), from a dry run only when the release was one.
//
// A missing, failed, stale or mismatched record wraps ErrMissingEvidence,
// naming the step ("test: ...", "docker: ..."); other errors (an
// unreadable or broken file) are returned as they are.
func CheckSyncEvidence(repo string, rel ReleaseOutputs) error {
	if rel.BaseCommit == "" {
		return fmt.Errorf("release: release.json has no base_commit (it was written by an older nerv): the tested commit cannot be checked: %w", ErrMissingEvidence)
	}
	if err := checkTestRecord(repo, rel); err != nil {
		return err
	}
	return checkDockerRecord(repo, rel)
}

func checkTestRecord(repo string, rel ReleaseOutputs) error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("test: "+format+": %w", append(args, ErrMissingEvidence)...)
	}
	r, err := ReadTestRecord(repo)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return refuse("no test record (%s): run nerv pipeline test at %s (the commit the release was made from) before syncing", TestRecordPath(repo), rel.BaseCommit)
	case err != nil:
		return fmt.Errorf("test: %w", err)
	case r.Status != StatusPassed:
		return refuse("the test record did not pass (status %q): run nerv pipeline test until it passes", r.Status)
	case r.Dirty:
		return refuse("tested uncommitted changes (%s): run nerv pipeline test again on a clean working tree at %s", strings.Join(r.Changes, ", "), rel.BaseCommit)
	case r.Commit != rel.BaseCommit:
		return refuse("the test record is for commit %s, but the release was made from %s: run nerv pipeline test at %s", r.Commit, rel.BaseCommit, rel.BaseCommit)
	}
	return nil
}

func checkDockerRecord(repo string, rel ReleaseOutputs) error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("docker: "+format+": %w", append(args, ErrMissingEvidence)...)
	}
	d, err := ReadDockerResult(repo)
	switch {
	case errors.Is(err, ErrNoDockerResult):
		return refuse("no docker.json (%s): run nerv pipeline docker until it passes before syncing", DockerResultPath(repo))
	case err != nil:
		return fmt.Errorf("docker: %w", err)
	case d.Status != StatusPassed:
		return refuse("docker.json did not pass (status %q): run nerv pipeline docker until it passes", d.Status)
	case d.Version != rel.Version || d.GitTag != rel.GitTag || d.Revision != rel.GitHead:
		return refuse("docker.json is for %s (%s, %s), not this release %s (%s, %s): run nerv pipeline docker for this release",
			d.Version, d.GitTag, d.Revision, rel.Version, rel.GitTag, rel.GitHead)
	case d.DryRun && !rel.DryRun:
		return refuse("docker.json comes from a dry run (nothing was pushed): run nerv pipeline docker without --dry-run")
	}
	return nil
}
