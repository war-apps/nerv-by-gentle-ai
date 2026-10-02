package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/war-apps/nerv-by-gentle-ai/internal/release"
)

// releaseUsage documents the maintainer-only "nerv release" subcommands.
// "nerv release" is deliberately absent from the top-level usage text (see
// the "Maintainer commands" line in main.go) -- it is release tooling for
// this repository, not something an installed user runs.
const releaseUsage = `Usage: nerv release <preview|apply|guard> [flags]

Maintainer-only release tooling: computes the next semantic version from
Conventional Commits since the last release tag (preview/apply), or checks
release readiness before tagging (guard). Exit codes: 0 ok, 1 refused,
2 environment/git error.

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

// runReleaseCompute parses preview/apply's flags, validates the ones that
// need no repository access (pre-release-base/label shape, the
// apply+pre-release exclusivity), delegates the actual version computation
// (and, for apply, the plugin.json/CHANGELOG writes) to internal/release,
// and prints the result. Every remaining validation (repository access,
// --version's format and ordering against the current version, the
// changelog/notes content itself) lives in internal/release, which owns
// this CLI's release-computation domain end to end.
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
	if *preRelease != "" && !release.IsValidPreReleaseLabel(*preRelease) {
		fmt.Fprintf(stdout, "Invalid --pre-release label %q; expected lowercase letters only (e.g. 'alpha', 'beta', 'rc').\n", *preRelease)
		return 1
	}
	if *preRelease != "" && apply {
		fmt.Fprintln(stdout, "--apply cannot be combined with --pre-release: pre-releases are tag-only and never modify plugin.json or CHANGELOG.md.")
		return 1
	}

	deps := release.Deps{Repo: *repo, Runner: opts.Runner, Now: opts.Now}
	previewOpts := release.PreviewOptions{Version: *versionFlag, PreRelease: *preRelease, PreReleaseBase: *preReleaseBase}

	compute := release.Preview
	if apply {
		compute = release.Apply
	}
	result, err := compute(deps, previewOpts)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return exitCodeFor(err)
	}

	if result.Next == nil {
		if *jsonOut {
			writeJSON(stdout, result)
		} else {
			fmt.Fprintf(stdout, "Nothing to release: %d commit(s) inspected since %s, none releasable.\n", result.InspectedCommits, result.Since)
		}
		return 1
	}

	if *jsonOut {
		writeJSON(stdout, result)
		return 0
	}

	printReleaseComputeText(stdout, result, *preRelease, apply)
	return 0
}

// printReleaseComputeText renders preview/apply's human-readable output.
func printReleaseComputeText(stdout io.Writer, result release.PreviewResult, preRelease string, apply bool) {
	fmt.Fprintf(stdout, "Current version : %s\n", result.Current)
	lastTagDisplay := "(none)"
	if result.LastTag != nil {
		lastTagDisplay = *result.LastTag
	}
	fmt.Fprintf(stdout, "Last tag        : %s\n", lastTagDisplay)
	fmt.Fprintf(stdout, "Bump            : %s\n", result.Bump)
	fmt.Fprintf(stdout, "Next version    : %s\n", *result.Next)
	fmt.Fprintf(stdout, "Tag             : %s\n", *result.Tag)
	if preRelease != "" {
		fmt.Fprintf(stdout, "Pre-release     : %s\n", preRelease)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, *result.Section)
	if apply {
		fmt.Fprintf(stdout, "Applied: wrote %s and %s\n", result.Written[0], result.Written[1])
	}
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

	deps := release.Deps{Repo: *repo, Runner: opts.Runner, Now: opts.Now, PluginFS: opts.PluginFS}
	result, err := release.Guard(deps, release.GuardOptions{NotesPath: *notesPath})
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return exitCodeFor(err)
	}

	if *jsonOut {
		writeJSON(stdout, result)
	} else if result.Ok {
		fmt.Fprintln(stdout, result.Version)
	} else {
		fmt.Fprintln(stdout, result.Reason)
	}

	if result.Ok {
		return 0
	}
	return 1
}
