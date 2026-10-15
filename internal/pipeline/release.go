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
	Error   string            `json:"error,omitempty"`
	// Action is what to do after a failure.
	Action string `json:"action,omitempty"`
}

// ReleaseOutputsPath is <repo>/.nerv/artifacts/pipeline/release.json.
func ReleaseOutputsPath(repo string) string {
	return filepath.Join(repo, ".nerv", "artifacts", "pipeline", ReleaseOutputsFile)
}

// WriteReleaseOutputs writes o to ReleaseOutputsPath (replacing it
// atomically), after the same checks as the test results: nothing is
// created through a symlink leading outside the repository.
func WriteReleaseOutputs(repo string, o ReleaseOutputs) (string, error) {
	path := ReleaseOutputsPath(repo)
	if err := prepareArtifact(repo, path); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".release-*.json")
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
func RemoveReleaseOutputs(repo string) error {
	path := ReleaseOutputsPath(repo)
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
		if _, err := fmt.Fprintf(p.w, "%s%s\n", p.prefix, p.buf[:i]); err != nil {
			return len(b), err
		}
		p.buf = p.buf[i+1:]
	}
}

// Flush writes a pending partial line.
func (p *PrefixWriter) Flush() error {
	if len(p.buf) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(p.w, "%s%s\n", p.prefix, p.buf)
	p.buf = nil
	return err
}
