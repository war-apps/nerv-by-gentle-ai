package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

const versionNextUsage = `Usage: nerv version next [flags]

Computes the next semantic version of the repository (or of each project of
the release: section of .ai-workflow.yml) with the repository's own
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
  --fetch-timeout <dur>    Bound for reading origin, positive (default 60s)
  --format text|json|env   Output format (default text)
  --json                   Same as --format json

The output shape follows the request: when the release: section declares
several projects and --project is not given, json is an array and env keys
are prefixed with the project name, "-" written as "_"
(api_new_release_version); otherwise (no section, one project, or
--project) json is one object and env keys are the output names, for that
project only. env: GitHub Actions output lines (key=value; key<<DELIMITER
for multi-line values). Progress goes to stderr; stdout carries only the
result. Exit codes: 0 ok (also when there is no new release), 1 refused
(invalid configuration, unknown project, not a git repository), 2 usage or
environment error, 128 + the signal when interrupted (130 Ctrl-C, 143
SIGTERM, 129 SIGHUP).
`

// versionFormats are the --format values.
var versionFormats = []string{"text", "json", "env"}

// signalNotify and signalStop are signal.Notify and signal.Stop; tests
// replace them to observe the handler's lifetime without real signals.
var (
	signalNotify = signal.Notify
	signalStop   = signal.Stop
)

// signalContext returns a context cancelled by the first of cancelSignals,
// a stop function (call it when the command returns), and received, which
// reports that signal (nil when none arrived).
//
// The signals stop being caught as soon as the context is done (by a signal,
// the parent or stop), before the cancellation is visible: a second Ctrl-C
// then gets the default behaviour and ends the process at once instead of
// being swallowed while the command winds down.
func signalContext(parent context.Context) (ctx context.Context, stop context.CancelFunc, received func() os.Signal) {
	ctx, cancel := context.WithCancel(parent)
	ch := make(chan os.Signal, 1)
	signalNotify(ch, cancelSignals...)
	var (
		mu     sync.Mutex
		got    os.Signal
		once   sync.Once
		closed = make(chan struct{})
	)
	release := func() { once.Do(func() { signalStop(ch); close(closed) }) }
	go func() {
		select {
		case sig := <-ch:
			mu.Lock()
			got = sig
			mu.Unlock()
			release()
			cancel()
		case <-ctx.Done():
			release()
		}
	}()
	stop = func() {
		cancel()
		<-closed
	}
	received = func() os.Signal {
		mu.Lock()
		defer mu.Unlock()
		return got
	}
	return ctx, stop, received
}

// interruptedExitCode is the shell convention for a process ended by sig:
// 128 + the signal number (SIGINT 130, SIGTERM 143, SIGHUP 129); 2 when the
// run was cancelled without a known signal.
func interruptedExitCode(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 2
}

func runVersionNext(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("version next", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cwd := fs.String("cwd", ".", "")
	branch := fs.String("branch", "", "")
	project := fs.String("project", "", "")
	offline := fs.Bool("offline", false, "")
	fetchTimeout := fs.Duration("fetch-timeout", semrel.DefaultFetchTimeout, "")
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
	if *fetchTimeout <= 0 {
		return versionNextUsageError(stderr, fmt.Sprintf("--fetch-timeout %s: must be positive (reading origin is always bounded; use --offline to skip it)", *fetchTimeout))
	}

	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop, received := signalCtx(context.Background())
	defer stop()
	report := func(err error) int { return reportVersionError(ctx, received(), stderr, err) }

	top, err := gitTopLevel(ctx, opts.Runner, *cwd)
	var shape outputShape
	if err == nil {
		shape, err = requestShape(top, *project)
	}
	if err != nil {
		return report(err)
	}
	if shape.Whole {
		workflowconfig.PrintHints(stderr, "", top)
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
		return report(err)
	}

	results, err = shape.selectResults(results)
	if err != nil {
		return report(err)
	}
	if err := writeVersionResult(stdout, *format, shape.Several, results); err != nil {
		return report(fmt.Errorf("writing the result: %w", err))
	}
	return 0
}

// outputShape is decided by the request, never by how many results the
// engine returned: Several (a JSON array, project-prefixed env keys) when
// the release: section declares more than one project and no --project is
// given; otherwise exactly one result: the project named Only, or, for a
// repository without a release: section (Whole), the single project the
// engine computed, whatever name it gave it.
type outputShape struct {
	Several bool
	Whole   bool
	Only    string
}

// selectResults keeps what the request asked for: every result for Several,
// otherwise only Only's (the engine also returns the dependencies it
// computed for depends_on). No result to print is an error.
func (s outputShape) selectResults(results []semrel.ProjectResult) ([]semrel.ProjectResult, error) {
	if s.Several {
		if len(results) == 0 {
			return nil, errors.New("the engine returned no result")
		}
		return results, nil
	}
	if s.Whole {
		if len(results) != 1 {
			return nil, fmt.Errorf("the engine returned no result for the repository (%d results, want 1)", len(results))
		}
		return results, nil
	}
	for _, r := range results {
		if r.Project == s.Only {
			return []semrel.ProjectResult{r}, nil
		}
	}
	return nil, fmt.Errorf("the engine returned no result for project %q", s.Only)
}

// writeVersionResult renders the results in format into memory, writes them
// in one call and flushes w when it buffers, so a failed or partial write is
// reported instead of lost.
func writeVersionResult(w io.Writer, format string, several bool, results []semrel.ProjectResult) error {
	var b bytes.Buffer
	switch format {
	case "json":
		var v any = results
		if !several {
			v = results[0]
		}
		encoded, err := json.Marshal(v)
		if err != nil {
			return err
		}
		b.Write(encoded)
		b.WriteByte('\n')
	case "env":
		if err := writeVersionEnv(&b, several, results); err != nil {
			return err
		}
	default:
		writeVersionText(&b, results)
	}
	if _, err := w.Write(b.Bytes()); err != nil {
		return err
	}
	if f, ok := w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

func versionNextUsageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "nerv version next: %s\n\n%s", problem, versionNextUsage)
	return 2
}

// reportVersionError prints err on stderr and returns its exit code: 1 for a
// refusal or an invalid release configuration, 128 + the signal for an
// interrupted run, 2 otherwise.
func reportVersionError(ctx context.Context, sig os.Signal, stderr io.Writer, err error) int {
	if ctx.Err() != nil {
		if sig != nil {
			fmt.Fprintf(stderr, "nerv version next: interrupted (%v)\n", sig)
		} else {
			fmt.Fprintln(stderr, "nerv version next: interrupted")
		}
		return interruptedExitCode(sig)
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

// requestShape loads the release: section once before the (slow) engine
// runs, so an invalid configuration or an unknown --project is refused at
// once, naming the projects that exist, and decides the output shape.
// Without a release: section the whole repository is the only project; the
// engine names it, so the CLI does not predict the name.
func requestShape(top, name string) (outputShape, error) {
	rel, err := shipconfig.Load(top)
	if errors.Is(err, shipconfig.ErrNotConfigured) {
		if name != "" {
			return outputShape{}, &refusal.Error{Err: errors.New(withLegacyHint(fmt.Sprintf("--project %q: the repository has no release: section in .ai-workflow.yml, so it is a single project; drop --project", name), top))}
		}
		return outputShape{Whole: true}, nil
	}
	if err != nil {
		return outputShape{}, err
	}
	if name == "" {
		if len(rel.Projects) == 1 {
			return outputShape{Only: rel.Projects[0].Name}, nil
		}
		return outputShape{Several: true}, nil
	}
	if _, ok := rel.Project(name); ok {
		return outputShape{Only: name}, nil
	}
	names := make([]string, 0, len(rel.Projects))
	for _, p := range rel.Projects {
		names = append(names, p.Name)
	}
	return outputShape{}, &refusal.Error{Err: fmt.Errorf("unknown project %q; the release: section declares: %s", name, strings.Join(names, ", "))}
}

// envKey is the env-format key of an output: the output name, prefixed with
// the project name ("-" written as "_") when the output holds several
// projects (see outputShape).
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
func writeVersionEnv(w io.Writer, prefixed bool, results []semrel.ProjectResult) error {
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

// configSourceText says where a result's semantic-release configuration
// came from: "repository (<file>)" or "built-in default (dim)".
func configSourceText(r semrel.ProjectResult) string {
	switch r.SemanticReleaseConfig {
	case semrel.ConfigRepository:
		return "repository (" + r.SemanticReleaseConfigFile + ")"
	case semrel.ConfigBuiltIn:
		return "built-in default (dim)"
	}
	return ""
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
		if s := configSourceText(r); s != "" {
			fmt.Fprintf(w, "  semantic-release config: %s\n", s)
		}
	}
}
