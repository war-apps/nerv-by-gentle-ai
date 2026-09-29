package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/release"
	"github.com/war-apps/nerv-gentle-ai/internal/version"
)

// releaseUsage documents the maintainer-only "nerv release" subcommands.
// "nerv release" is deliberately absent from the top-level usage text (see
// the "Maintainer commands" line in main.go) -- it is release tooling for
// this repository, not something an installed user runs.
const releaseUsage = `Usage: nerv release <preview|apply|guard> [flags]

Maintainer-only release tooling, replacing tools/release.ps1 and
tools/release-guard.ps1: computes the next semantic version from
Conventional Commits since the last release tag (preview/apply), or checks
release readiness before tagging (guard). Same output shapes and exit codes
as the scripts (0 ok, 1 refused, 2 environment/git error).

preview/apply flags:
  --repo <dir>                      Repository root (default ".")
  --version X.Y.Z                   Explicit version override
  --pre-release <label>             Pre-release label (e.g. alpha, beta, rc)
  --pre-release-base next|current   What --pre-release bumps from (default "next")
  --json                            Print the result as JSON

guard flags:
  --repo <dir>     Repository root (default ".")
  --notes <path>   Write the changelog section body here on success
  --json           Print the result as JSON
`

var (
	semverExactPattern     = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	semverPrefixPattern    = regexp.MustCompile(`^(\d+\.\d+\.\d+)`)
	preReleaseLabelPattern = regexp.MustCompile(`^[a-z]+$`)
)

func runRelease(args []string, stdout, stderr io.Writer, opts options) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, releaseUsage)
		return 2
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "--help", "-h", "help":
		fmt.Fprint(stdout, releaseUsage)
		return 0
	case "preview":
		return runReleaseCompute(rest, stdout, opts, false)
	case "apply":
		return runReleaseCompute(rest, stdout, opts, true)
	case "guard":
		return runReleaseGuard(rest, stdout, opts)
	default:
		fmt.Fprint(stderr, releaseUsage)
		return 2
	}
}

// releaseCommitPayload is one commit in the "commits" JSON array, matching
// tools/release.ps1's ordered hashtable fields.
type releaseCommitPayload struct {
	Sha      string `json:"sha"`
	ShortSha string `json:"short_sha"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

// releaseResult mirrors tools/release.ps1's -Json output shape and field
// order: current, last_tag, bump, next, tag, base, prerelease, commits,
// section, applied, written (written only present when applied).
type releaseResult struct {
	Current    string                 `json:"current"`
	LastTag    *string                `json:"last_tag"`
	Bump       string                 `json:"bump"`
	Next       *string                `json:"next"`
	Tag        *string                `json:"tag"`
	Base       *string                `json:"base"`
	PreRelease *string                `json:"prerelease"`
	Commits    []releaseCommitPayload `json:"commits"`
	Section    *string                `json:"section"`
	Applied    bool                   `json:"applied"`
	Written    []string               `json:"written,omitempty"`
}

func runReleaseCompute(args []string, stdout io.Writer, opts options, apply bool) int {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	repo := fs.String("repo", ".", "")
	versionFlag := fs.String("version", "", "")
	preRelease := fs.String("pre-release", "", "")
	preReleaseBase := fs.String("pre-release-base", "next", "")
	jsonOut := fs.Bool("json", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprint(stdout, releaseUsage)
		return 2
	}

	if *preReleaseBase != "next" && *preReleaseBase != "current" {
		fmt.Fprintf(stdout, "Invalid --pre-release-base %q; expected \"next\" or \"current\".\n", *preReleaseBase)
		return 1
	}
	if *preReleaseBase != "next" && *preRelease == "" {
		fmt.Fprintln(stdout, "--pre-release-base requires --pre-release; it has no meaning on its own.")
		return 1
	}
	if *preRelease != "" && !preReleaseLabelPattern.MatchString(*preRelease) {
		fmt.Fprintf(stdout, "Invalid --pre-release label %q; expected lowercase letters only (e.g. 'alpha', 'beta', 'rc').\n", *preRelease)
		return 1
	}
	if *preRelease != "" && apply {
		fmt.Fprintln(stdout, "--apply cannot be combined with --pre-release: pre-releases are tag-only and never modify plugin.json or CHANGELOG.md.")
		return 1
	}

	info, statErr := os.Stat(*repo)
	if statErr != nil || !info.IsDir() {
		fmt.Fprintf(stdout, "Repository path not found: %q.\n", *repo)
		return 2
	}

	pluginJSONPath := filepath.Join(*repo, "plugin", ".claude-plugin", "plugin.json")
	pluginJSONBytes, err := os.ReadFile(pluginJSONPath)
	if err != nil {
		fmt.Fprintf(stdout, "plugin.json not found at %q.\n", pluginJSONPath)
		return 2
	}

	currentVersion, err := release.PluginVersion(string(pluginJSONBytes))
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}

	ctx := context.Background()
	allTags, err := release.Tags(ctx, opts.Runner, *repo, "v*")
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}
	lastTag := release.LastReleaseTag(allTags, false)

	commits, err := release.CommitsSince(ctx, opts.Runner, *repo, lastTag)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}
	bumpKind := release.BumpKind(commits)

	var nextVersion string
	switch {
	case *versionFlag != "":
		if !semverExactPattern.MatchString(*versionFlag) {
			fmt.Fprintf(stdout, "Invalid --version %q; expected semver X.Y.Z.\n", *versionFlag)
			return 1
		}
		if !semverGreater(*versionFlag, currentVersion) {
			fmt.Fprintf(stdout, "--version %q must be strictly greater than the current version %q.\n", *versionFlag, currentVersion)
			return 1
		}
		nextVersion = *versionFlag
	case *preRelease != "":
		nextVersion, err = release.NextVersion(currentVersion, bumpKind, *preRelease, *preReleaseBase, allTags)
		if err != nil {
			fmt.Fprintf(stdout, "nerv: %v\n", err)
			return 1
		}
	default:
		nextVersion, err = release.NextVersion(currentVersion, bumpKind, "", "", nil)
		if err != nil {
			fmt.Fprintf(stdout, "nerv: %v\n", err)
			return 2
		}
	}

	dateStr := opts.Now().Format("2006-01-02")
	var section string
	if nextVersion != "" {
		section = release.ChangelogSection(nextVersion, dateStr, commits)
	}

	commitsPayload := make([]releaseCommitPayload, 0, len(commits))
	for _, cm := range commits {
		commitsPayload = append(commitsPayload, releaseCommitPayload{Sha: cm.Sha, ShortSha: cm.ShortSha, Subject: cm.Subject, Body: cm.Body})
	}

	result := releaseResult{Current: currentVersion, Bump: bumpKind, Commits: commitsPayload}
	if lastTag != "" {
		lt := lastTag
		result.LastTag = &lt
	}
	if *preRelease != "" {
		pr := *preRelease
		result.PreRelease = &pr
	}

	changelogPath := filepath.Join(*repo, "CHANGELOG.md")

	if nextVersion == "" {
		if *jsonOut {
			writeJSON(stdout, result)
		} else {
			since := "the repository root"
			if lastTag != "" {
				since = lastTag
			}
			fmt.Fprintf(stdout, "Nothing to release: %d commit(s) inspected since %s, none releasable.\n", len(commits), since)
		}
		return 1
	}

	nv := nextVersion
	result.Next = &nv
	sec := section
	result.Section = &sec
	tagValue := "v" + nextVersion
	result.Tag = &tagValue
	baseVersion := semverPrefixOf(nextVersion)
	result.Base = &baseVersion

	if apply {
		newPluginJSON, _, err := release.SetPluginVersion(string(pluginJSONBytes), nextVersion)
		if err != nil {
			fmt.Fprintf(stdout, "nerv: %v\n", err)
			return 2
		}
		if err := os.WriteFile(pluginJSONPath, []byte(newPluginJSON), 0o644); err != nil {
			fmt.Fprintf(stdout, "nerv: writing %s: %v\n", pluginJSONPath, err)
			return 2
		}

		changelogContent := ""
		if existing, err := os.ReadFile(changelogPath); err == nil {
			changelogContent = string(existing)
		}
		newChangelog, _, err := release.UpdateChangelog(changelogContent, section)
		if err != nil {
			fmt.Fprintf(stdout, "nerv: %v\n", err)
			return 2
		}
		if err := os.WriteFile(changelogPath, []byte(newChangelog), 0o644); err != nil {
			fmt.Fprintf(stdout, "nerv: writing %s: %v\n", changelogPath, err)
			return 2
		}

		result.Applied = true
		result.Written = []string{pluginJSONPath, changelogPath}
	}

	if *jsonOut {
		writeJSON(stdout, result)
		return 0
	}

	fmt.Fprintf(stdout, "Current version : %s\n", currentVersion)
	lastTagDisplay := "(none)"
	if lastTag != "" {
		lastTagDisplay = lastTag
	}
	fmt.Fprintf(stdout, "Last tag        : %s\n", lastTagDisplay)
	fmt.Fprintf(stdout, "Bump            : %s\n", bumpKind)
	fmt.Fprintf(stdout, "Next version    : %s\n", nextVersion)
	fmt.Fprintf(stdout, "Tag             : %s\n", tagValue)
	if *preRelease != "" {
		fmt.Fprintf(stdout, "Pre-release     : %s\n", *preRelease)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, section)
	if apply {
		fmt.Fprintf(stdout, "Applied: wrote %s and %s\n", pluginJSONPath, changelogPath)
	}

	return 0
}

// releaseGuardResult mirrors tools/release-guard.ps1's -Json output shape,
// plus one new field this Go port adds: binary_consistent (see
// runReleaseGuard's doc comment).
type releaseGuardResult struct {
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
	// plugin.json version in --repo -- a consistency assertion that the
	// binary running this guard was actually built from this tree.
	BinaryConsistent *bool `json:"binary_consistent,omitempty"`
}

func runReleaseGuard(args []string, stdout io.Writer, opts options) int {
	fs := flag.NewFlagSet("release guard", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	repo := fs.String("repo", ".", "")
	notesPath := fs.String("notes", "", "")
	jsonOut := fs.Bool("json", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprint(stdout, releaseUsage)
		return 2
	}

	info, statErr := os.Stat(*repo)
	if statErr != nil || !info.IsDir() {
		fmt.Fprintf(stdout, "Repository path not found: %q.\n", *repo)
		return 2
	}

	pluginJSONPath := filepath.Join(*repo, "plugin", ".claude-plugin", "plugin.json")
	pluginJSONBytes, err := os.ReadFile(pluginJSONPath)
	if err != nil {
		fmt.Fprintf(stdout, "plugin.json not found at %q.\n", pluginJSONPath)
		return 2
	}
	pluginVersion, err := release.PluginVersion(string(pluginJSONBytes))
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}

	tag := "v" + pluginVersion
	tags, err := release.Tags(context.Background(), opts.Runner, *repo, tag)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}

	changelogPath := filepath.Join(*repo, "CHANGELOG.md")
	changelogExists := false
	changelogText := ""
	if data, err := os.ReadFile(changelogPath); err == nil {
		changelogExists = true
		changelogText = string(data)
	}

	readiness := release.Readiness(pluginVersion, tags, changelogExists, changelogText)

	ok := readiness.Ok
	reason := readiness.Reason

	var binaryConsistent *bool
	if version.Binary != "dev" {
		embeddedVersion, err := version.PluginVersion(opts.PluginFS)
		if err != nil {
			fmt.Fprintf(stdout, "nerv: %v\n", err)
			return 2
		}
		consistent := embeddedVersion == pluginVersion
		binaryConsistent = &consistent
		if !consistent && ok {
			ok = false
			reason = fmt.Sprintf("embedded plugin version %q does not match on-disk plugin.json version %q; this binary was not built from this tree", embeddedVersion, pluginVersion)
		}
	}

	if ok && *notesPath != "" {
		if dir := filepath.Dir(*notesPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Fprintf(stdout, "nerv: creating %s: %v\n", dir, err)
				return 2
			}
		}
		if err := os.WriteFile(*notesPath, []byte(readiness.Body+"\n"), 0o644); err != nil {
			fmt.Fprintf(stdout, "nerv: writing %s: %v\n", *notesPath, err)
			return 2
		}
	}

	result := releaseGuardResult{
		Version:          readiness.Version,
		Tag:              readiness.Tag,
		ChangelogSection: readiness.ChangelogSection,
		TagExists:        readiness.TagExists,
		Ok:               ok,
		BinaryConsistent: binaryConsistent,
	}
	if *notesPath != "" {
		result.NotesPath = notesPath
	}

	if *jsonOut {
		writeJSON(stdout, result)
	} else if ok {
		fmt.Fprintln(stdout, readiness.Version)
	} else {
		fmt.Fprintln(stdout, reason)
	}

	if ok {
		return 0
	}
	return 1
}

func semverGreater(a, b string) bool {
	ap, bp := strings.Split(a, "."), strings.Split(b, ".")
	if len(ap) < 3 || len(bp) < 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		ai, _ := strconv.Atoi(ap[i])
		bi, _ := strconv.Atoi(bp[i])
		if ai > bi {
			return true
		}
		if ai < bi {
			return false
		}
	}
	return false
}

func semverPrefixOf(s string) string {
	m := semverPrefixPattern.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	return m[1]
}
