package release

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
)

// Deps bundles the release use cases' external effects: the repository
// directory every path is resolved under, the process-launching seam for
// git, the clock used to date changelog sections and read notes, and the
// embedded plugin tree Guard compares against the on-disk plugin.json.
type Deps struct {
	Repo     string
	Runner   env.Runner
	Now      func() time.Time
	PluginFS fs.FS
}

// PreviewOptions are Preview/Apply's shared inputs, one per CLI flag of the
// same name.
type PreviewOptions struct {
	// Version overrides the computed next version; it must be strictly
	// greater than the current version. Empty means "compute it".
	Version string
	// PreRelease is a pre-release label (e.g. "alpha", "beta", "rc");
	// empty means a normal release.
	PreRelease string
	// PreReleaseBase selects what PreRelease bumps from: "next" (default,
	// empty string also means "next") or "current".
	PreReleaseBase string
}

// CommitPayload is one commit in a PreviewResult's Commits.
type CommitPayload struct {
	Sha      string `json:"sha"`
	ShortSha string `json:"short_sha"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

// PreviewResult is Preview/Apply's outcome: current, last_tag, bump, next,
// tag, base, prerelease, commits, section, applied, written (written only
// present when applied).
type PreviewResult struct {
	Current    string          `json:"current"`
	LastTag    *string         `json:"last_tag"`
	Bump       string          `json:"bump"`
	Next       *string         `json:"next"`
	Tag        *string         `json:"tag"`
	Base       *string         `json:"base"`
	PreRelease *string         `json:"prerelease"`
	Commits    []CommitPayload `json:"commits"`
	Section    *string         `json:"section"`
	Applied    bool            `json:"applied"`
	Written    []string        `json:"written,omitempty"`

	// InspectedCommits and Since report on the "nothing to release" case
	// (Next == nil): the caller formats its own text/JSON around them, but
	// does not need to recompute them. Excluded from the JSON shape
	// (json:"-") so preview --json's output carries only the fields above.
	InspectedCommits int    `json:"-"`
	Since            string `json:"-"`
}

// Preview computes the next version and changelog section from opts
// without writing anything. A nil error with Next == nil means nothing is
// releasable (bump is "none" and neither Version nor PreRelease forced
// one) -- an expected outcome the caller maps to its own "nothing to
// release" reporting and exit code, not a failure.
func Preview(deps Deps, opts PreviewOptions) (PreviewResult, error) {
	return compute(deps, opts, false)
}

// Apply computes exactly like Preview, then -- when a next version was
// found -- writes it into plugin.json and inserts the new section into
// CHANGELOG.md (creating it from the standard Keep-a-Changelog header when
// it does not exist yet).
func Apply(deps Deps, opts PreviewOptions) (PreviewResult, error) {
	return compute(deps, opts, true)
}

func compute(deps Deps, opts PreviewOptions, apply bool) (PreviewResult, error) {
	pluginJSONPath := filepath.Join(deps.Repo, "plugin", ".claude-plugin", "plugin.json")
	pluginJSONBytes, currentVersion, err := readPluginVersion(deps.Repo, pluginJSONPath)
	if err != nil {
		return PreviewResult{}, err
	}

	ctx := context.Background()
	allTags, err := Tags(ctx, deps.Runner, deps.Repo, "v*")
	if err != nil {
		return PreviewResult{}, err
	}
	lastTag := LastReleaseTag(allTags)

	commits, err := CommitsSince(ctx, deps.Runner, deps.Repo, lastTag)
	if err != nil {
		return PreviewResult{}, err
	}
	bumpKind := BumpKind(commits)

	nextVersion, err := resolveNextVersion(opts, currentVersion, bumpKind, allTags)
	if err != nil {
		return PreviewResult{}, err
	}

	result := PreviewResult{Current: currentVersion, Bump: bumpKind, Commits: commitPayloads(commits)}
	if lastTag != "" {
		result.LastTag = &lastTag
	}
	if opts.PreRelease != "" {
		pr := opts.PreRelease
		result.PreRelease = &pr
	}

	if nextVersion == "" {
		since := "the repository root"
		if lastTag != "" {
			since = lastTag
		}
		result.InspectedCommits = len(commits)
		result.Since = since
		return result, nil
	}

	dateStr := deps.Now().Format("2006-01-02")
	section := ChangelogSection(nextVersion, dateStr, commits)
	tag := "v" + nextVersion
	base := PrefixOf(nextVersion)
	result.Next = &nextVersion
	result.Section = &section
	result.Tag = &tag
	result.Base = &base

	if apply {
		changelogPath := filepath.Join(deps.Repo, "CHANGELOG.md")
		written, err := applyVersion(pluginJSONPath, string(pluginJSONBytes), nextVersion, changelogPath, section)
		if err != nil {
			return result, err
		}
		result.Applied = true
		result.Written = written
	}

	return result, nil
}

// readPluginVersion reads and validates plugin.json under repo, returning
// its raw bytes (for Apply's targeted regex rewrite) alongside the parsed
// version.
func readPluginVersion(repo, pluginJSONPath string) (raw []byte, currentVersion string, err error) {
	info, statErr := os.Stat(repo)
	if statErr != nil || !info.IsDir() {
		return nil, "", fmt.Errorf("repository path not found: %q", repo)
	}

	raw, err = os.ReadFile(pluginJSONPath)
	if err != nil {
		return nil, "", fmt.Errorf("plugin.json not found at %q", pluginJSONPath)
	}

	currentVersion, err = PluginVersion(string(raw))
	if err != nil {
		return nil, "", err
	}
	return raw, currentVersion, nil
}

// resolveNextVersion picks the next version from opts.Version (an
// explicit override), opts.PreRelease (a pre-release bump), or the
// computed bumpKind -- in that priority order.
func resolveNextVersion(opts PreviewOptions, currentVersion, bumpKind string, allTags []string) (string, error) {
	switch {
	case opts.Version != "":
		if !IsExactSemver(opts.Version) {
			return "", &refusal.Error{Err: fmt.Errorf("invalid --version %q; expected semver X.Y.Z", opts.Version)}
		}
		if !Greater(opts.Version, currentVersion) {
			return "", &refusal.Error{Err: fmt.Errorf("--version %q must be strictly greater than the current version %q", opts.Version, currentVersion)}
		}
		return opts.Version, nil

	case opts.PreRelease != "":
		next, err := NextVersion(currentVersion, bumpKind, opts.PreRelease, opts.PreReleaseBase, allTags)
		if err != nil {
			return "", &refusal.Error{Err: err}
		}
		return next, nil

	default:
		return NextVersion(currentVersion, bumpKind, "", "", nil)
	}
}

func commitPayloads(commits []Commit) []CommitPayload {
	payloads := make([]CommitPayload, 0, len(commits))
	for _, cm := range commits {
		payloads = append(payloads, CommitPayload{Sha: cm.Sha, ShortSha: cm.ShortSha, Subject: cm.Subject, Body: cm.Body})
	}
	return payloads
}

// applyVersion writes nextVersion into pluginJSONContent at pluginJSONPath
// and inserts section into the changelog at changelogPath (reading its
// existing content first; a missing file is treated as "does not exist
// yet", matching UpdateChangelog's own contract), returning the paths
// written.
func applyVersion(pluginJSONPath, pluginJSONContent, nextVersion, changelogPath, section string) ([]string, error) {
	newPluginJSON, _, err := SetPluginVersion(pluginJSONContent, nextVersion)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(pluginJSONPath, []byte(newPluginJSON), 0o644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", pluginJSONPath, err)
	}

	changelogContent := ""
	if existing, err := os.ReadFile(changelogPath); err == nil {
		changelogContent = string(existing)
	}
	newChangelog, _, err := UpdateChangelog(changelogContent, section)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(changelogPath, []byte(newChangelog), 0o644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", changelogPath, err)
	}

	return []string{pluginJSONPath, changelogPath}, nil
}

// GuardResult is Guard's outcome: version, tag, changelog_section,
// tag_exists, notes_path, ok, plus BinaryConsistent (see Guard's doc
// comment).
type GuardResult struct {
	Version          string  `json:"version"`
	Tag              string  `json:"tag"`
	ChangelogSection bool    `json:"changelog_section"`
	TagExists        bool    `json:"tag_exists"`
	NotesPath        *string `json:"notes_path"`
	Ok               bool    `json:"ok"`
	// BinaryConsistent is nil (omitted) when version.Binary == "dev": a
	// local dev build was never built from a specific tagged tree, so the
	// check is meaningless. Otherwise it reports whether the embedded
	// plugin version (baked in at build time) matches the on-disk
	// plugin.json version in Deps.Repo -- a consistency assertion that the
	// binary running this guard was actually built from this tree.
	BinaryConsistent *bool `json:"binary_consistent,omitempty"`
	// Reason is the human-readable refusal reason, printed in text mode
	// only -- excluded from the JSON shape (json:"-").
	Reason string `json:"-"`
}

// GuardOptions is Guard's one input: where to write the changelog
// section body on success (empty means "don't write a notes file").
type GuardOptions struct {
	NotesPath string
}

// Guard checks whether the plugin version in Deps.Repo is ready to
// release: the tag "v<version>" must not already exist, and CHANGELOG.md
// must contain a matching "## [<version>]" section. On success, when
// opts.NotesPath is non-empty, that section's body is written there.
// Guard itself never returns an error for a normal "not ready" outcome
// (Result.Ok reports that) -- only for an environment failure (a missing
// plugin.json, a git launch failure, a notes-file write failure).
func Guard(deps Deps, opts GuardOptions) (GuardResult, error) {
	pluginJSONPath := filepath.Join(deps.Repo, "plugin", ".claude-plugin", "plugin.json")
	_, pluginVersion, err := readPluginVersion(deps.Repo, pluginJSONPath)
	if err != nil {
		return GuardResult{}, err
	}

	tag := "v" + pluginVersion
	tags, err := Tags(context.Background(), deps.Runner, deps.Repo, tag)
	if err != nil {
		return GuardResult{}, err
	}

	changelogPath := filepath.Join(deps.Repo, "CHANGELOG.md")
	changelogExists := false
	changelogText := ""
	if data, err := os.ReadFile(changelogPath); err == nil {
		changelogExists = true
		changelogText = string(data)
	}

	readiness := Readiness(pluginVersion, tags, changelogExists, changelogText)
	ok, reason := readiness.Ok, readiness.Reason

	var binaryConsistent *bool
	if version.Binary != "dev" {
		embeddedVersion, err := version.PluginVersion(deps.PluginFS)
		if err != nil {
			return GuardResult{}, err
		}
		consistent := embeddedVersion == pluginVersion
		binaryConsistent = &consistent
		if !consistent && ok {
			ok = false
			reason = fmt.Sprintf("embedded plugin version %q does not match on-disk plugin.json version %q; this binary was not built from this tree", embeddedVersion, pluginVersion)
		}
	}

	if ok && opts.NotesPath != "" {
		if err := writeNotesFile(opts.NotesPath, readiness.Body); err != nil {
			return GuardResult{}, err
		}
	}

	result := GuardResult{
		Version:          readiness.Version,
		Tag:              readiness.Tag,
		ChangelogSection: readiness.ChangelogSection,
		TagExists:        readiness.TagExists,
		Ok:               ok,
		Reason:           reason,
		BinaryConsistent: binaryConsistent,
	}
	if opts.NotesPath != "" {
		result.NotesPath = &opts.NotesPath
	}
	return result, nil
}

func writeNotesFile(path, body string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(body+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
