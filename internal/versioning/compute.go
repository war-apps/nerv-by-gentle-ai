// Package versioning computes the next release of a repository, or of each
// project of a monorepo, natively in Go: the behaviour of semantic-release
// with the conventionalcommits preset that nerv relies on (commit analysis,
// release branches with prerelease identifiers and channels, tag formats,
// per-project commit filters, depends_on, and the release notes), without
// node, npm or semantic-release.
//
// The sources it reproduces are semantic-release 24.2,
// @semantic-release/commit-analyzer 13, @semantic-release/release-notes-
// generator 14, conventional-commits-parser 6, conventional-changelog-writer
// 8 and conventional-changelog-conventionalcommits 8; each file names the
// parts it ports. Git is only read, through an injectable env.Runner.
package versioning

import (
	"strconv"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
)

// Request selects what Compute computes.
type Request struct {
	// RepoDir is any directory inside the repository.
	RepoDir string
	// Branch is the branch to compute; empty means the current one.
	Branch string
	// Projects restricts the results to these projects (their dependencies
	// are still computed, to apply depends_on); empty means every project.
	Projects []string
	// Config is the engine's configuration; its defaults are applied.
	Config Config
	// Offline uses the local tags and release notes as they are. Otherwise
	// origin's tags and semantic-release notes are fetched first, as
	// semantic-release does.
	Offline bool
	// Runner runs git; nil means env.ExecRunner.
	Runner env.Runner
	// Now is the clock of the release date in the notes; nil means
	// time.Now. The date is written in UTC.
	Now func() time.Time
}

// Result is the next release of one project on one branch: the 12 outputs
// of cycjimmy/semantic-release-action (as semrel.ProjectResult), plus the
// project, the branch, the release type, the CHANGELOG section and, when
// there is no release, the reason. The last_release_* fields are filled
// whether or not there is a new release.
type Result struct {
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

	// Bump is the release type: major, minor or patch; empty without a
	// release.
	Bump string `json:"bump,omitempty"`
	// Changelog is the project's CHANGELOG file, repository-relative.
	Changelog string `json:"changelog"`
	// ChangelogSection is what @semantic-release/changelog prepends to the
	// CHANGELOG for this release (see PrependChangelog).
	ChangelogSection string `json:"changelog_section,omitempty"`
	// Reason says why there is no release.
	Reason string `json:"reason,omitempty"`
}

// Output is one named action output.
type Output struct {
	Name  string
	Value string
}

// Outputs returns the 12 action outputs in the action's order.
func (r Result) Outputs() []Output {
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
