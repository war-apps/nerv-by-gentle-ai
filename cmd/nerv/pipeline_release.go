package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// pipelineReleaseName names the command in the shared ship messages.
const pipelineReleaseName = "nerv pipeline release"

// runPipelineRelease is `nerv pipeline release`: dim's release job. It
// releases what is on origin exactly as nerv ship does (release clone,
// local-vs-origin check, push check, the repository's semantic-release
// configuration or the built-in one, version guard, --expect, --dry-run,
// confirmation), without ship's build, publish or hooks: it writes
// pipeline.release.version_files, runs semantic-release, and leaves the
// outputs in .nerv/artifacts/pipeline/release.json for the later steps.
func runPipelineRelease(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("pipeline release", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	s := &shipRun{opts: opts, stdout: io.Discard, releaseOnly: true, expect: expectations{}}
	fs.StringVar(&s.cwd, "cwd", ".", "")
	fs.StringVar(&s.branch, "branch", "", "")
	fs.BoolVar(&s.dryRun, "dry-run", false, "")
	fs.BoolVar(&s.yes, "yes", false, "")
	fs.DurationVar(&s.timeout, "fetch-timeout", semrel.DefaultFetchTimeout, "")
	fs.Var(s.expect, "expect", "")
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

	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop, received := signalCtx(context.Background())
	defer stop()

	// Every line on stderr is "[pipeline:release] ...", and the shared ship
	// messages name this command.
	prefixed := pipeline.NewPrefixWriter(renameWriter{stderr}, "[pipeline:release] ")
	defer prefixed.Flush()
	s.stderr = prefixed

	doc := pipeline.ReleaseOutputs{Step: "release", DryRun: s.dryRun, Channel: ""}
	code := 0
	top, err := s.prepareRelease(ctx, opts.OpenShipWorkspace)
	if err == nil {
		var outcomes []shipOutcome
		outcomes, err = s.execute(ctx)
		fillReleaseOutputs(&doc, s, outcomes)
	}
	if err != nil {
		var status string
		code, status, doc.Error = classifyShipError(ctx, received(), prefixed, err)
		doc.Status = map[string]string{"declined": "refused"}[status]
		if doc.Status == "" {
			doc.Status = status
		}
	} else {
		doc.Status = pipeline.StatusPassed
		write := opts.WriteReleaseOutputs
		if write == nil {
			write = pipeline.WriteReleaseOutputs
		}
		if path, werr := write(top, doc); werr != nil {
			code = 2
			outputsWriteFailed(prefixed, &doc, werr)
		} else {
			fmt.Fprintf(prefixed, "%s: outputs in %s\n", pipelineReleaseName, path)
		}
	}
	doc.Error, doc.Action = renamed(doc.Error), renamed(doc.Action)
	prefixed.Flush()
	if werr := writeReleaseResult(stdout, *jsonOut, doc); werr != nil && code == 0 {
		fmt.Fprintf(stderr, "%s: writing the result: %v\n", pipelineReleaseName, werr)
		code = 2
	}
	return code
}

// prepareRelease loads the local pipeline: section (required) and its
// version files, refuses a release: section with several projects (this
// step releases one), and removes an earlier release.json so a failed run
// never leaves stale outputs. It returns the repository's top level.
//
// base is the caller's workspace opener (nil: semrel.NewWorkspace), never
// s.opts' current one: the release opener is built from it, so preparing
// again does not wrap it twice.
func (s *shipRun) prepareRelease(ctx context.Context, base func(context.Context, semrel.WorkspaceOptions) (shipWorkspace, error)) (string, error) {
	top, err := gitTopLevel(ctx, s.opts.Runner, s.cwd)
	if err != nil {
		return "", err
	}
	cfg, err := pipelineconfig.Load(top)
	switch {
	case errors.Is(err, pipelineconfig.ErrNotConfigured):
		return "", refuse("the repository has no pipeline: section in .nerv/nerv.yaml; see docs/pipeline.md")
	case err != nil:
		// An invalid section is a refusal (exit 1), as for nerv ship.
		return "", &refusal.Error{Err: err}
	}
	if rel, err := shipconfig.Load(top); err == nil && len(rel.Projects) > 1 {
		return "", refuse("the release: section declares %d projects; nerv pipeline release releases one project (use nerv ship for a monorepo)", len(rel.Projects))
	}
	s.versionFiles = cfg.Release.VersionFiles
	// git and semantic-release in the release clone never get the registry
	// or deploy secrets (GitHub tokens stay: the github plugin needs them).
	s.opts.OpenShipWorkspace = func(ctx context.Context, o semrel.WorkspaceOptions) (shipWorkspace, error) {
		o.Env = pipeline.SafeEnv(os.Environ(), cfg.Registry)
		var ws shipWorkspace
		var err error
		if base != nil {
			ws, err = base(ctx, o)
		} else {
			ws, err = semrel.NewWorkspace(ctx, o)
		}
		if err != nil {
			return nil, err
		}
		return taggedCheck{ws}, nil
	}
	if err := pipeline.RemoveReleaseOutputs(top); err != nil {
		return "", err
	}
	return top, nil
}

// outputsWriteFailed reports a release.json that could not be written. In
// a real run the release is already on origin: the document says what was
// released, the later steps' need for release.json, and the recovery (the
// exact content, printed on one line, saved by hand), never a rerun.
func outputsWriteFailed(stderr io.Writer, doc *pipeline.ReleaseOutputs, werr error) {
	want := *doc
	content, _ := json.Marshal(want)
	doc.Status, doc.Error = pipeline.StatusFailed, "writing "+pipeline.ReleaseOutputsFile+": "+werr.Error()
	fmt.Fprintf(stderr, "%s: %s\n", pipelineReleaseName, doc.Error)
	if doc.DryRun || !doc.Published {
		doc.Action = "Nothing was released. Fix the cause and run the step again."
	} else {
		doc.Action = fmt.Sprintf("The release is on origin: version %s, tag %s, commit %s. The docker, sync and deploy steps read %s, which could not be written. Fix the cause, then save the line printed after %q on stderr as that file. Do not run nerv pipeline release again for this release: it would compute the next version.",
			doc.Version, doc.GitTag, doc.GitHead, ".nerv/artifacts/pipeline/"+pipeline.ReleaseOutputsFile, "release.json should hold: ")
	}
	fmt.Fprintf(stderr, "%s: release.json should hold: %s\n", pipelineReleaseName, content)
}

// fillReleaseOutputs copies the released (or, in a dry run, the computed)
// values into doc; a failed real run keeps the computation under Computed
// and reports nothing as published.
func fillReleaseOutputs(doc *pipeline.ReleaseOutputs, s *shipRun, outcomes []shipOutcome) {
	if len(s.results) == 0 {
		return
	}
	r := s.results[0]
	if s.released.NewReleasePublished {
		r = s.released
	} else if !s.dryRun && r.NewReleasePublished {
		// Computed but not released (refused, failed or interrupted).
		doc.Computed = &pipeline.ComputedRelease{Version: r.NewReleaseVersion, Channel: r.NewReleaseChannel, GitTag: r.NewReleaseGitTag}
		r.NewReleasePublished = false
	}
	doc.Project, doc.Branch, doc.LastVersion = r.Project, r.Branch, r.LastReleaseVersion
	doc.SemanticReleaseConfig = s.results[0].SemanticReleaseConfig
	doc.VersionFiles = s.versionFiles
	doc.Published = r.NewReleasePublished
	if r.NewReleasePublished {
		doc.Version, doc.Channel, doc.GitTag, doc.GitHead = r.NewReleaseVersion, r.NewReleaseChannel, r.NewReleaseGitTag, r.NewReleaseGitHead
		// semantic-release ran on origin's head in the release clone.
		if s.ws != nil {
			doc.BaseCommit = s.ws.OriginHead()
		}
	}
	doc.Outputs = map[string]string{}
	for _, o := range r.Outputs() {
		doc.Outputs[o.Name] = o.Value
	}
	if len(outcomes) > 0 {
		doc.Action = outcomes[0].Action
	}
}

// renameWriter rewrites the shared ship messages to name this command; it
// receives whole lines (from the PrefixWriter).
type renameWriter struct{ w io.Writer }

func (r renameWriter) Write(b []byte) (int, error) {
	if _, err := io.WriteString(r.w, renamed(string(b))); err != nil {
		return 0, err
	}
	return len(b), nil
}

func renamed(text string) string { return strings.ReplaceAll(text, "nerv ship", pipelineReleaseName) }

// writeReleaseResult writes the release document: JSON, or a short text.
func writeReleaseResult(w io.Writer, asJSON bool, doc pipeline.ReleaseOutputs) error {
	var b bytes.Buffer
	if asJSON {
		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
	} else {
		fmt.Fprintf(&b, "release: %s\n", doc.Status)
		switch {
		case doc.Status != pipeline.StatusPassed:
		case !doc.Published:
			fmt.Fprintf(&b, "  no new release (last %s)\n", orNone(doc.LastVersion))
		default:
			what := "released"
			if doc.DryRun {
				what = "dry run: would release"
			}
			channel := doc.Channel
			if channel == "" {
				channel = "default"
			}
			fmt.Fprintf(&b, "  %s %s (channel %s, tag %s)\n", what, doc.Version, channel, doc.GitTag)
		}
		if doc.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", doc.Error)
		}
		if doc.Action != "" {
			fmt.Fprintf(&b, "  %s\n", doc.Action)
		}
	}
	_, err := w.Write(b.Bytes())
	return err
}

// taggedCheck refuses a computed release whose tag is already on origin,
// in a dry run and a real one alike: semantic-release computes it again
// when it cannot see that tag's release on the branch's channel, usually
// because origin's refs/notes/semantic-release-<tag> notes are stale or
// missing.
type taggedCheck struct{ shipWorkspace }

func (w taggedCheck) Compute(ctx context.Context, project string) ([]semrel.ProjectResult, error) {
	results, err := w.shipWorkspace.Compute(ctx, project)
	if err != nil {
		return results, err
	}
	for _, r := range results {
		if !r.NewReleasePublished || r.NewReleaseGitTag == "" {
			continue
		}
		exists, err := w.TagOnOrigin(ctx, r.NewReleaseGitTag)
		if err != nil {
			return nil, fmt.Errorf("checking whether %s is on origin: %w", r.NewReleaseGitTag, err)
		}
		if exists {
			return nil, refuse("the computed release %s: tag %s is already on origin: semantic-release did not see that release on the %s branch's channel, usually because origin's semantic-release notes (refs/notes/semantic-release-%s) are stale or missing; fetch them (git fetch origin 'refs/notes/*:refs/notes/*') or restore them on origin, then run again. Nothing was released.",
				r.NewReleaseVersion, r.NewReleaseGitTag, r.Branch, r.NewReleaseGitTag)
		}
	}
	return results, nil
}
