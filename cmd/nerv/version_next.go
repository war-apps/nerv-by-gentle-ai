package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"os/signal"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

const versionNextUsage = `Usage: nerv version next [flags]

Computes the next semantic version of the repository (or of each project of
the release: section of .nerv/nerv.yaml) with the repository's own
semantic-release configuration, in dry-run mode. Nothing in the repository
changes. Needs Node.js and npm (semantic-release is installed in a NERV
cache) and git.

Flags:
  --cwd <dir>              A directory inside the repository (default ".")
  --branch <name>          Local branch to compute (default: the current one)
  --project <name>         Only this project of the release: section
  --offline                Use the local tags and notes only; by default
                            origin's release tags and notes are read first
                            (with your git credentials, into a throwaway
                            copy)
  --fetch-timeout <dur>    Bound for reading origin (default 60s)
  --format text|json|env   Output format (default text)
  --json                   Same as --format json

json: one object for one result (a single project, or --project), an array
when several projects are computed. env: GitHub Actions output lines
(key=value; key<<DELIMITER for multi-line values); with several projects
each key is prefixed with the project name, "-" written as "_"
(api_new_release_version). Progress goes to stderr; stdout carries only the
result. Exit codes: 0 ok (also when there is no new release), 1 refused
(invalid configuration, unknown project, not a git repository), 2 usage or
environment error.
`

// versionFormats are the --format values.
var versionFormats = []string{"text", "json", "env"}

// signalContext cancels ctx on the signals that should stop a running
// command (see cancelSignals).
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, cancelSignals...)
}

func runVersionNext(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("version next", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cwd := fs.String("cwd", ".", "")
	branch := fs.String("branch", "", "")
	project := fs.String("project", "", "")
	offline := fs.Bool("offline", false, "")
	fetchTimeout := fs.Duration("fetch-timeout", 0, "")
	format := fs.String("format", "text", "")
	jsonOut := fs.Bool("json", false, "")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, versionNextUsage)
			return 0
		}
		return versionNextUsageError(stderr, err.Error())
	}
	if fs.NArg() > 0 {
		return versionNextUsageError(stderr, fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}
	if !slices.Contains(versionFormats, *format) {
		return versionNextUsageError(stderr, fmt.Sprintf("--format %q: want one of %s", *format, strings.Join(versionFormats, ", ")))
	}
	if *jsonOut {
		formatSet := false
		fs.Visit(func(f *flag.Flag) { formatSet = formatSet || f.Name == "format" })
		if formatSet && *format != "json" {
			return versionNextUsageError(stderr, fmt.Sprintf("--json conflicts with --format %s", *format))
		}
		*format = "json"
	}
	if *fetchTimeout < 0 {
		return versionNextUsageError(stderr, "--fetch-timeout must not be negative")
	}

	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop := signalCtx(context.Background())
	defer stop()

	top, err := gitTopLevel(ctx, opts.Runner, *cwd)
	if err == nil {
		err = checkProject(top, *project)
	}
	if err != nil {
		return reportVersionError(ctx, stderr, err)
	}

	source := "reading origin's release tags and notes"
	if *offline {
		source = "offline: local tags and notes only"
	}
	target := "the current branch"
	if *branch != "" {
		target = "branch " + *branch
	}
	fmt.Fprintf(stderr, "nerv version next: computing %s of %s (%s)\n", target, top, source)

	compute := opts.ComputeVersions
	if compute == nil {
		compute = semrel.Compute
	}
	results, err := compute(ctx, semrel.Options{
		RepoRoot:     top,
		Branch:       *branch,
		Project:      *project,
		Offline:      *offline,
		FetchTimeout: *fetchTimeout,
	})
	if err != nil {
		return reportVersionError(ctx, stderr, err)
	}

	switch *format {
	case "json":
		if len(results) == 1 {
			writeJSON(stdout, results[0])
		} else {
			writeJSON(stdout, results)
		}
	case "env":
		if err := writeVersionEnv(stdout, results); err != nil {
			return reportVersionError(ctx, stderr, err)
		}
	default:
		writeVersionText(stdout, results)
	}
	return 0
}

func versionNextUsageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "nerv version next: %s\n\n%s", problem, versionNextUsage)
	return 2
}

// reportVersionError prints err on stderr and returns its exit code: 1 for a
// refusal or an invalid release configuration, 2 otherwise.
func reportVersionError(ctx context.Context, stderr io.Writer, err error) int {
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "nerv version next: interrupted")
		return 2
	}
	fmt.Fprintf(stderr, "nerv version next: %v\n", err)
	if errors.Is(err, semrel.ErrOriginFetch) {
		fmt.Fprintln(stderr, "Run again with --offline to compute from the local tags and notes only (a release channel recorded only on origin may then be missed).")
	}
	var invalid *shipconfig.ValidationError
	if errors.As(err, &invalid) {
		return 1
	}
	return exitCodeFor(err)
}

// gitTopLevel resolves dir to the top level of its git repository. A
// directory outside any repository is a refusal; a git that cannot run is
// an environment error.
func gitTopLevel(ctx context.Context, runner env.Runner, dir string) (string, error) {
	out, errOut, code, err := runner.Run(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", errors.New("git is required (git was not found in PATH)")
		}
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	if code != 0 {
		detail := strings.TrimSpace(errOut)
		if detail != "" {
			detail = " (" + detail + ")"
		}
		return "", &refusal.Error{Err: fmt.Errorf("%s is not inside a git repository%s", dir, detail)}
	}
	return strings.TrimSpace(out), nil
}

// checkProject loads the release: section once before the (slow) engine
// runs, so an invalid configuration or an unknown --project is refused at
// once, naming the projects that exist.
func checkProject(top, name string) error {
	rel, err := shipconfig.Load(top)
	if errors.Is(err, shipconfig.ErrNotConfigured) {
		if name != "" {
			return &refusal.Error{Err: fmt.Errorf("--project %q: the repository has no release: section in .nerv/nerv.yaml, so it is a single project; drop --project", name)}
		}
		return nil
	}
	if err != nil {
		return err
	}
	if name == "" {
		return nil
	}
	if _, ok := rel.Project(name); ok {
		return nil
	}
	names := make([]string, 0, len(rel.Projects))
	for _, p := range rel.Projects {
		names = append(names, p.Name)
	}
	return &refusal.Error{Err: fmt.Errorf("unknown project %q; the release: section declares: %s", name, strings.Join(names, ", "))}
}

// envKey is the env-format key of an output: the output name, prefixed with
// the project name ("-" written as "_") when several projects are printed.
func envKey(project, output string, prefixed bool) string {
	if !prefixed {
		return output
	}
	return strings.ReplaceAll(project, "-", "_") + "_" + output
}

// writeVersionEnv writes the results in GitHub Actions output syntax.
// Multi-line values use the heredoc form with a random delimiter that does
// not occur in the value, written as @actions/core does: the value, a
// newline, then the delimiter.
func writeVersionEnv(w io.Writer, results []semrel.ProjectResult) error {
	prefixed := len(results) > 1
	var b strings.Builder
	for _, r := range results {
		for _, o := range r.Outputs() {
			key := envKey(r.Project, o.Name, prefixed)
			if !strings.ContainsAny(o.Value, "\r\n") {
				fmt.Fprintf(&b, "%s=%s\n", key, o.Value)
				continue
			}
			delim, err := heredocDelimiter(o.Value)
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, "%s<<%s\n%s\n%s\n", key, delim, o.Value, delim)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func heredocDelimiter(value string) (string, error) {
	for {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		delim := "ghadelimiter_" + hex.EncodeToString(raw)
		if !strings.Contains(value, delim) {
			return delim, nil
		}
	}
}

// writeVersionText writes a short human summary per project.
func writeVersionText(w io.Writer, results []semrel.ProjectResult) {
	for i, r := range results {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s (%s)\n", r.Project, r.Branch)
		if r.NewReleasePublished {
			channel := "default channel"
			if r.NewReleaseChannel != "" {
				channel = "channel " + r.NewReleaseChannel
			}
			fmt.Fprintf(w, "  next release: %s (%s, tag %s)\n", r.NewReleaseVersion, channel, r.NewReleaseGitTag)
		} else {
			fmt.Fprintln(w, "  next release: no new release")
		}
		if r.LastReleaseVersion != "" {
			fmt.Fprintf(w, "  last release: %s (tag %s)\n", r.LastReleaseVersion, r.LastReleaseGitTag)
		} else {
			fmt.Fprintln(w, "  last release: none")
		}
	}
}
