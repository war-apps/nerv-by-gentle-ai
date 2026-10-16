package pipeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ReleaseOutputsFile is where the release step leaves its outputs for the
// later steps (docker, sync, deploy), under the pipeline artifacts.
const ReleaseOutputsFile = "release.json"

// ReleaseOutputs is the release step's result: dim's release outputs
// (scripts/set-release-outputs.sh: new-release-published, release-version,
// channel, git-tag) plus semantic-release-action's 12 outputs. It is what
// `nerv pipeline release --json` prints and what release.json holds.
type ReleaseOutputs struct {
	Step   string `json:"step"`
	Status string `json:"status"`
	// DryRun: computed only, nothing released.
	DryRun bool `json:"dry_run"`
	// Published is dim's new-release-published.
	Published bool   `json:"published"`
	Project   string `json:"project,omitempty"`
	Branch    string `json:"branch,omitempty"`
	// Version is dim's release-version.
	Version string `json:"version,omitempty"`
	// Channel is dim's channel: empty on the default channel.
	Channel string `json:"channel"`
	// GitTag is dim's git-tag.
	GitTag string `json:"git_tag,omitempty"`
	// GitHead is the released commit (the release commit when a plugin made
	// one).
	GitHead     string `json:"git_head,omitempty"`
	LastVersion string `json:"last_version,omitempty"`
	// SemanticReleaseConfig is "repository" or "default" (built-in).
	SemanticReleaseConfig string `json:"semantic_release_config,omitempty"`
	// VersionFiles are the project files versioned in the release commit.
	VersionFiles []string `json:"version_files,omitempty"`
	// Outputs are semantic-release-action's 12 outputs.
	Outputs map[string]string `json:"outputs,omitempty"`
	// Computed is the release computed before semantic-release ran, when
	// the run did not release it (a failure); Published and the release
	// fields above only describe what was actually released.
	Computed *ComputedRelease `json:"computed,omitempty"`
	Error    string           `json:"error,omitempty"`
	// Action is what to do after a failure.
	Action string `json:"action,omitempty"`
}

// ComputedRelease is a release computed but not (or not known to be) made.
type ComputedRelease struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
	GitTag  string `json:"git_tag"`
}

// ReleaseOutputsPath is <repo>/.nerv/artifacts/pipeline/release.json.
func ReleaseOutputsPath(repo string) string {
	return filepath.Join(repo, ".nerv", "artifacts", "pipeline", ReleaseOutputsFile)
}

// WriteReleaseOutputs writes o to ReleaseOutputsPath (replacing it
// atomically), after the same checks as the test results: nothing is
// created through a symlink leading outside the repository.
func WriteReleaseOutputs(repo string, o ReleaseOutputs) (string, error) {
	return writeArtifactJSON(repo, ReleaseOutputsPath(repo), o)
}

// ErrNoReleaseOutputs is wrapped by ReadReleaseOutputs when there is no
// release.json (the release step has not run, or its last run failed).
var ErrNoReleaseOutputs = errors.New("no release outputs")

// ReadReleaseOutputs reads release.json, after the artifacts' symlink
// checks.
func ReadReleaseOutputs(repo string) (ReleaseOutputs, error) {
	var o ReleaseOutputs
	path := ReleaseOutputsPath(repo)
	err := readArtifactJSON(repo, path, &o)
	if errors.Is(err, os.ErrNotExist) {
		return o, fmt.Errorf("%s: %w", path, ErrNoReleaseOutputs)
	}
	return o, err
}

// readArtifactJSON decodes path under the artifacts into v, after the
// symlink checks; a missing file is an error wrapping os.ErrNotExist.
func readArtifactJSON(repo, path string, v any) error {
	artifacts := filepath.Join(repo, ".nerv", "artifacts")
	if err := checkAncestorInside(repo, artifacts); err != nil {
		return err
	}
	if _, err := os.Lstat(artifacts); err == nil {
		if err := checkRealInside(repo, artifacts, path); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// DockerResultFile is the docker step's result, for the deploy step.
const DockerResultFile = "docker.json"

// DockerResultPath is <repo>/.nerv/artifacts/pipeline/docker.json.
func DockerResultPath(repo string) string {
	return filepath.Join(repo, ".nerv", "artifacts", "pipeline", DockerResultFile)
}

// WriteDockerResult writes docker.json as WriteReleaseOutputs writes
// release.json.
func WriteDockerResult(repo string, r DockerResult) (string, error) {
	return writeArtifactJSON(repo, DockerResultPath(repo), r)
}

// RemoveDockerResult removes an earlier docker.json.
func RemoveDockerResult(repo string) error { return removeArtifact(repo, DockerResultPath(repo)) }

// writeArtifactJSON writes v as indented JSON to path under the artifacts,
// atomically, after the symlink checks.
func writeArtifactJSON(repo, path string, v any) (string, error) {
	if err := prepareArtifact(repo, path); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".artifact-*.json")
	if err != nil {
		return "", err
	}
	_, werr := tmp.Write(append(data, '\n'))
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil {
		os.Remove(tmp.Name())
		return "", werr
	}
	return path, nil
}

// RemoveReleaseOutputs removes an earlier release.json, so a failed or
// interrupted release never leaves stale outputs for the later steps.
func RemoveReleaseOutputs(repo string) error { return removeArtifact(repo, ReleaseOutputsPath(repo)) }

// removeArtifact removes path under the artifacts after the symlink checks.
func removeArtifact(repo, path string) error {
	artifacts := filepath.Join(repo, ".nerv", "artifacts")
	if err := checkAncestorInside(repo, artifacts); err != nil {
		return err
	}
	if _, err := os.Lstat(artifacts); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := checkRealInside(repo, artifacts, path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// prepareArtifact makes path's directory under .nerv/artifacts (self-
// ignored) after checking, symlinks resolved, that nothing leads outside
// the repository.
func prepareArtifact(repo, path string) error {
	artifacts := filepath.Join(repo, ".nerv", "artifacts")
	if rel, err := filepath.Rel(artifacts, path); err != nil || !filepath.IsLocal(rel) {
		return fmt.Errorf("%s is not inside %s", path, artifacts)
	}
	if err := checkAncestorInside(repo, artifacts); err != nil {
		return err
	}
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		return err
	}
	if err := checkRealInside(repo, artifacts, path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ignore := filepath.Join(artifacts, ".gitignore")
	if _, err := os.Stat(ignore); errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(ignore, []byte("*\n"), 0o644)
	}
	return nil
}

// PrefixWriter prefixes every line written through it (the
// "[pipeline:<step>] " output prefix); a partial line waits for its end or
// Flush.
type PrefixWriter struct {
	w      io.Writer
	prefix string
	buf    []byte
}

// NewPrefixWriter returns a PrefixWriter writing to w.
func NewPrefixWriter(w io.Writer, prefix string) *PrefixWriter {
	return &PrefixWriter{w: w, prefix: prefix}
}

func (p *PrefixWriter) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			return len(b), nil
		}
		line := p.buf[:i]
		// The line is consumed before writing: a line that failed is
		// dropped, never written again by the next Write or Flush.
		p.buf = p.buf[i+1:]
		if _, err := fmt.Fprintf(p.w, "%s%s\n", p.prefix, line); err != nil {
			return len(b), err
		}
	}
}

// Flush writes a pending partial line.
func (p *PrefixWriter) Flush() error {
	if len(p.buf) == 0 {
		return nil
	}
	line := p.buf
	p.buf = nil
	_, err := fmt.Fprintf(p.w, "%s%s\n", p.prefix, line)
	return err
}
