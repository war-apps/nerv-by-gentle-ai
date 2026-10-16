package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
)

// syncWorkspace is the clone the sync step merges in; *semrel.Workspace is
// the real one.
type syncWorkspace interface {
	pipeline.SyncRepo
	CheckPush(ctx context.Context) error
	UseCommitIdentity(ctx context.Context) (string, error)
	Close() error
}

var errSyncDeclined = errors.New("cancelled: nothing was merged or pushed")

// runPipelineSync is `nerv pipeline sync`: dim's sync-to-develop and
// sync-main-to-release jobs after the release step, in a fresh clone of
// origin, committing as the user and pushing with the user's credentials.
func runPipelineSync(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("pipeline sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	s := syncRun{opts: opts}
	cwd := fs.String("cwd", ".", "")
	fs.BoolVar(&s.dryRun, "dry-run", false, "")
	fs.BoolVar(&s.yes, "yes", false, "")
	fs.DurationVar(&s.timeout, "fetch-timeout", semrel.DefaultFetchTimeout, "")
	jsonOut := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, pipelineUsage)
			return 0
		}
		return pipelineUsageError(stderr, err.Error())
	}
	if fs.NArg() > 0 {
		return pipelineUsageError(stderr, fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}
	if s.timeout <= 0 {
		return pipelineUsageError(stderr, fmt.Sprintf("--fetch-timeout %s: must be positive", s.timeout))
	}
	s.cwd = *cwd
	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop, received := signalCtx(context.Background())
	defer stop()

	out := pipeline.NewPrefixWriter(stderr, "[pipeline:sync] ")
	defer out.Flush()
	s.out = out
	res, top, err := s.run(ctx)
	code, write := 0, false
	switch {
	case ctx.Err() != nil:
		res.Status, code = pipeline.StatusInterrupted, interruptedExitCode(received())
		// A run interrupted during the merges still records the targets
		// that finished (pushed ones are on origin) and those not run.
		write = err == nil && len(res.Branches) > 0
		fmt.Fprintln(out, "nerv pipeline sync: interrupted")
	case err != nil:
		res.Status, res.Error, code = pipeline.StatusFailed, err.Error(), 2
		if refusal.Is(err) {
			res.Status, code = pipelineRefused, 1
		}
		fmt.Fprintf(out, "nerv pipeline sync: %v\n", err)
	case res.Status != pipeline.StatusPassed:
		code, write = 2, true
		fmt.Fprintf(out, "nerv pipeline sync: %s\n", res.Error)
		if res.Action != "" {
			fmt.Fprintf(out, "nerv pipeline sync: %s\n", res.Action)
		}
	default:
		write = true
	}
	// sync.json records every run that reached the merges (or found nothing
	// to do), failed merges included; never a refusal.
	if write {
		if path, werr := pipeline.WriteSyncResult(top, res); werr != nil {
			res.Status, res.Error, code = pipeline.StatusFailed, "writing "+pipeline.SyncResultFile+": "+werr.Error(), 2
			fmt.Fprintf(out, "nerv pipeline sync: %s\n", res.Error)
		} else {
			fmt.Fprintf(out, "nerv pipeline sync: result in %s\n", path)
		}
	}
	out.Flush()
	if werr := writeSyncResult(stdout, *jsonOut, res); werr != nil && code == 0 {
		code = 2
	}
	return code
}

type syncRun struct {
	opts        options
	out         io.Writer
	cwd         string
	dryRun, yes bool
	timeout     time.Duration
}

// run checks the inputs, opens the clone, confirms and syncs; it returns the
// result, the repository's top level and an error for a refusal or a
// failure before the merges.
func (s syncRun) run(ctx context.Context) (pipeline.SyncResult, string, error) {
	res := pipeline.SyncResult{Step: "sync", DryRun: s.dryRun, Branches: []pipeline.BranchSync{}}
	top, err := gitTopLevel(ctx, s.opts.Runner, s.cwd)
	if err != nil {
		return res, "", err
	}
	cfg, err := pipelineconfig.Load(top)
	switch {
	case errors.Is(err, pipelineconfig.ErrNotConfigured):
		return res, top, refuse("the repository has no pipeline: section in .nerv/nerv.yaml; see docs/pipeline.md")
	case err != nil:
		return res, top, &refusal.Error{Err: err}
	}
	if err := pipeline.RemoveSyncResult(top); err != nil {
		return res, top, err
	}
	if !cfg.Sync.IsEnabled() {
		res.Status = pipeline.StatusPassed
		fmt.Fprintln(s.out, "nerv pipeline sync: pipeline.sync.enabled is false: nothing to do")
		return res, top, nil
	}
	rel, err := pipeline.ReadReleaseOutputs(top)
	switch {
	case errors.Is(err, pipeline.ErrNoReleaseOutputs):
		return res, top, refuse("no release outputs (%s): run nerv pipeline release first", pipeline.ReleaseOutputsPath(top))
	case err != nil:
		return res, top, err
	case rel.Status != pipeline.StatusPassed:
		return res, top, refuse("the last release step did not pass (status %s): run nerv pipeline release first", rel.Status)
	}
	res.Version, res.Source = rel.Version, rel.Branch
	if !rel.Published {
		res.Status = pipeline.StatusPassed
		fmt.Fprintln(s.out, "nerv pipeline sync: no new release: nothing to sync")
		return res, top, nil
	}
	if rel.DryRun && !s.dryRun {
		return res, top, refuse("release.json comes from a dry run (%s was not released): run nerv pipeline release without --dry-run first, or nerv pipeline sync --dry-run", rel.Version)
	}
	// Sync nothing unless every earlier step passed for this release.
	if err := pipeline.CheckSyncEvidence(top, rel); err != nil {
		if errors.Is(err, pipeline.ErrMissingEvidence) {
			return res, top, &refusal.Error{Err: err}
		}
		return res, top, err
	}

	open := s.opts.OpenSyncWorkspace
	if open == nil {
		open = func(ctx context.Context, o semrel.WorkspaceOptions) (syncWorkspace, error) {
			return semrel.NewWorkspace(ctx, o)
		}
	}
	fmt.Fprintf(s.out, "nerv pipeline sync: reading origin's branches into a sync clone\n")
	ws, err := open(ctx, semrel.WorkspaceOptions{RepoRoot: top, Branch: rel.Branch, FetchTimeout: s.timeout})
	if errors.Is(err, semrel.ErrNoOrigin) {
		return res, top, refuse("the repository has no origin remote")
	}
	if err != nil {
		return res, top, err
	}
	defer ws.Close()
	sync := pipeline.Sync{Repo: ws, Config: cfg, Release: rel, Output: s.out, DryRun: s.dryRun}
	targets, err := sync.Targets(ctx)
	if err != nil {
		return res, top, err
	}
	if len(targets) == 0 {
		res.Status = pipeline.StatusPassed
		fmt.Fprintf(s.out, "nerv pipeline sync: a release on %s merges into nothing: nothing to sync (only %s and %s sync back)\n", rel.Branch, cfg.MainBranch(), cfg.Sync.Release)
		return res, top, nil
	}
	ident, err := ws.UseCommitIdentity(ctx)
	if errors.Is(err, semrel.ErrNoCommitIdentity) {
		return res, top, &refusal.Error{Err: err}
	}
	if err != nil {
		return res, top, err
	}
	for _, t := range targets {
		fmt.Fprintf(s.out, "nerv pipeline sync: merge %s into %s and push\n", t.Source, t.Branch)
	}
	fmt.Fprintf(s.out, "nerv pipeline sync: merge commits by %s\n", ident)
	if !s.dryRun {
		if err := ws.CheckPush(ctx); err != nil {
			return res, top, err
		}
		if err := s.confirm(len(targets)); err != nil {
			return res, top, err
		}
	}
	out := sync.Run(ctx, targets)
	out.Identity = ident
	return out, top, nil
}

// confirm asks before pushing on an interactive terminal; --yes skips it,
// and without a terminal --yes is required.
func (s syncRun) confirm(n int) error {
	if s.yes {
		return nil
	}
	if s.opts.IsTerminal == nil || !s.opts.IsTerminal() || s.opts.Stdin == nil {
		return refuse("confirmation needed but stdin is not an interactive terminal: pass --yes to merge and push without asking (or --dry-run to preview)")
	}
	fmt.Fprintf(s.out, "Merge and push %d branch(es)? [y/N]: ", n)
	line, sawNewline := readAnswer(s.opts.Stdin)
	if !sawNewline {
		fmt.Fprintln(s.out)
	}
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		return &refusal.Error{Err: errSyncDeclined}
	}
	return nil
}

// writeSyncResult writes the result: JSON, or a short text per branch.
func writeSyncResult(w io.Writer, asJSON bool, r pipeline.SyncResult) error {
	var b bytes.Buffer
	if asJSON {
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
	} else {
		fmt.Fprintf(&b, "sync: %s\n", r.Status)
		for _, br := range r.Branches {
			fmt.Fprintf(&b, "  %s <- %s: %s", br.Branch, br.Source, br.Status)
			if br.Commit != "" {
				fmt.Fprintf(&b, " (%s)", short(br.Commit))
			}
			if len(br.Conflicts) > 0 {
				fmt.Fprintf(&b, " in %s", strings.Join(br.Conflicts, ", "))
			}
			if br.Error != "" {
				fmt.Fprintf(&b, ": %s", br.Error)
			}
			b.WriteByte('\n')
		}
		if r.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", r.Error)
		}
	}
	_, err := w.Write(b.Bytes())
	return err
}
