package semrel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// ProjectResult is the next release of one project on one branch, as the 12
// outputs of cycjimmy/semantic-release-action plus the project and branch.
// Unlike the action, the last_release_* fields are also filled when there is
// no new release.
type ProjectResult struct {
	Project string `json:"project"`
	Branch  string `json:"branch"`

	NewReleasePublished    bool   `json:"new_release_published"`
	NewReleaseVersion      string `json:"new_release_version"`
	NewReleaseMajorVersion string `json:"new_release_major_version"`
	NewReleaseMinorVersion string `json:"new_release_minor_version"`
	NewReleasePatchVersion string `json:"new_release_patch_version"`
	// NewReleaseChannel is empty for the default channel.
	NewReleaseChannel  string `json:"new_release_channel"`
	NewReleaseNotes    string `json:"new_release_notes"`
	NewReleaseGitHead  string `json:"new_release_git_head"`
	NewReleaseGitTag   string `json:"new_release_git_tag"`
	LastReleaseVersion string `json:"last_release_version"`
	LastReleaseGitHead string `json:"last_release_git_head"`
	LastReleaseGitTag  string `json:"last_release_git_tag"`
}

// Output is one named action output.
type Output struct {
	Name  string
	Value string
}

// Outputs returns the 12 action outputs in the action's order.
func (r ProjectResult) Outputs() []Output {
	return []Output{
		{"new_release_published", strconv.FormatBool(r.NewReleasePublished)},
		{"new_release_version", r.NewReleaseVersion},
		{"new_release_major_version", r.NewReleaseMajorVersion},
		{"new_release_minor_version", r.NewReleaseMinorVersion},
		{"new_release_patch_version", r.NewReleasePatchVersion},
		{"new_release_channel", r.NewReleaseChannel},
		{"new_release_notes", r.NewReleaseNotes},
		{"new_release_git_head", r.NewReleaseGitHead},
		{"new_release_git_tag", r.NewReleaseGitTag},
		{"last_release_version", r.LastReleaseVersion},
		{"last_release_git_head", r.LastReleaseGitHead},
		{"last_release_git_tag", r.LastReleaseGitTag},
	}
}

// resultMarker prefixes the runner's single result line on stdout.
const resultMarker = "NERV_SEMREL_RESULT "

// runnerRequest is the JSON the node runner reads on stdin.
type runnerRequest struct {
	Mode          string `json:"mode"`
	CacheDir      string `json:"cacheDir"`
	Cwd           string `json:"cwd"`
	RepositoryURL string `json:"repositoryUrl,omitempty"`
	// TagFormat overrides the repository's tagFormat; empty keeps it.
	TagFormat string        `json:"tagFormat,omitempty"`
	Filter    *commitFilter `json:"filter"`
	// MinReleaseType is the lowest release type (patch, minor) the run
	// returns, from released dependencies; empty means none.
	MinReleaseType string `json:"minReleaseType,omitempty"`
	// CommitFilesCache is a file the runner keeps the files of each commit
	// in, shared by the projects of one run (one git listing per commit).
	CommitFilesCache string `json:"commitFilesCache,omitempty"`
}

// commitFilter keeps the commits that touch Paths or carry one of Scopes.
type commitFilter struct {
	Paths  []string `json:"paths"`
	Scopes []string `json:"scopes"`
}

// runnerResponse is the runner's result line.
type runnerResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	// plan mode
	Packages   []string `json:"packages"`
	Unresolved []string `json:"unresolved"`
	// run mode
	Released    bool     `json:"released"`
	NextRelease *release `json:"nextRelease"`
	LastRelease *release `json:"lastRelease"`
}

type release struct {
	Type    string `json:"type"`
	Version string `json:"version"`
	Channel string `json:"channel"`
	GitHead string `json:"gitHead"`
	GitTag  string `json:"gitTag"`
	Notes   string `json:"notes"`
}

// parseResponse reads the last result line of the runner's stdout.
func parseResponse(stdout, stderr []byte) (runnerResponse, error) {
	var line []byte
	for _, l := range bytes.Split(stdout, []byte("\n")) {
		if rest, ok := bytes.CutPrefix(bytes.TrimRight(l, "\r"), []byte(resultMarker)); ok {
			line = rest
		}
	}
	var resp runnerResponse
	if line == nil {
		return resp, fmt.Errorf("the semantic-release runner returned no result%s", stderrTail(stderr))
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return resp, fmt.Errorf("the semantic-release runner returned an unreadable result: %w", err)
	}
	if !resp.OK {
		msg := resp.Error
		if msg == "" {
			msg = "unknown error"
		}
		return resp, errors.New("semantic-release: " + redact(msg))
	}
	return resp, nil
}

// normalize turns a run response into the action outputs.
func normalize(project, branch string, resp runnerResponse) ProjectResult {
	r := ProjectResult{Project: project, Branch: branch}
	if l := resp.LastRelease; l != nil {
		r.LastReleaseVersion, r.LastReleaseGitHead, r.LastReleaseGitTag = l.Version, l.GitHead, l.GitTag
	}
	n := resp.NextRelease
	if !resp.Released || n == nil {
		return r
	}
	r.NewReleasePublished = true
	r.NewReleaseVersion = n.Version
	r.NewReleaseMajorVersion, r.NewReleaseMinorVersion, r.NewReleasePatchVersion = versionParts(n.Version)
	r.NewReleaseChannel = n.Channel
	r.NewReleaseNotes = n.Notes
	r.NewReleaseGitHead = n.GitHead
	r.NewReleaseGitTag = n.GitTag
	return r
}

// versionParts returns the major, minor and patch of a semantic version
// ("2.9.0-alpha.10" -> "2", "9", "0").
func versionParts(v string) (string, string, string) {
	core, _, _ := strings.Cut(v, "-")
	core, _, _ = strings.Cut(core, "+")
	parts := strings.SplitN(core, ".", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	return parts[0], parts[1], parts[2]
}

// releaseTypes is semantic-release's RELEASE_TYPE order.
var releaseTypes = []string{"patch", "minor", "major"}

// minReleaseType is the lowest release type a project gets from its
// dependencies released in this run: the highest bump among them, ignoring
// bump none and unreleased dependencies; empty when none applies.
func minReleaseType(deps []shipconfig.Dependency, results map[string]ProjectResult) string {
	best := -1
	for _, d := range deps {
		if d.Bump == "none" || !results[d.Project].NewReleasePublished {
			continue
		}
		best = max(best, slices.Index(releaseTypes, d.Bump))
	}
	if best < 0 {
		return ""
	}
	return releaseTypes[best]
}
