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
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// dockerWorkspace is the clone the docker step builds in; *semrel.Workspace
// is the real one.
type dockerWorkspace interface {
	Dir() string
	PublicURL() string
	OriginHead() string
	CheckoutTag(ctx context.Context, tag string) (string, error)
	Close() error
}

// runPipelineDocker is `nerv pipeline docker`: dim's docker job, from the
// release step's release.json, in a fresh clone of origin at the release
// tag.
func runPipelineDocker(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("pipeline docker", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cwd := fs.String("cwd", ".", "")
	dryRun := fs.Bool("dry-run", false, "")
	noLogin := fs.Bool("no-login", false, "")
	timeout := fs.Duration("fetch-timeout", semrel.DefaultFetchTimeout, "")
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
	if *timeout <= 0 {
		return pipelineUsageError(stderr, fmt.Sprintf("--fetch-timeout %s: must be positive", *timeout))
	}
	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop, received := signalCtx(context.Background())
	defer stop()

	out := pipeline.NewPrefixWriter(stderr, "[pipeline:docker] ")
	defer out.Flush()
	d := dockerRun{opts: opts, out: out, cwd: *cwd, dryRun: *dryRun, noLogin: *noLogin, timeout: *timeout}
	res, top, err := d.run(ctx)
	code := 0
	switch {
	case ctx.Err() != nil:
		res.Status, code = pipeline.StatusInterrupted, interruptedExitCode(received())
		fmt.Fprintln(out, "nerv pipeline docker: interrupted")
	case err != nil:
		res.Status, res.Error, code = pipeline.StatusFailed, err.Error(), 2
		if refusal.Is(err) {
			res.Status, code = pipelineRefused, 1
		}
		fmt.Fprintf(out, "nerv pipeline docker: %v\n", err)
	case res.Status != pipeline.StatusPassed:
		code = 2
		fmt.Fprintf(out, "nerv pipeline docker: %s\n", res.Error)
	default:
		if path, werr := pipeline.WriteDockerResult(top, res); werr != nil {
			res.Status, res.Error, code = pipeline.StatusFailed, "writing "+pipeline.DockerResultFile+": "+werr.Error(), 2
			fmt.Fprintf(out, "nerv pipeline docker: %s\n", res.Error)
		} else {
			fmt.Fprintf(out, "nerv pipeline docker: result in %s\n", path)
		}
	}
	out.Flush()
	if werr := writeDockerResult(stdout, *jsonOut, res); werr != nil && code == 0 {
		code = 2
	}
	return code
}

type dockerRun struct {
	opts            options
	out             io.Writer
	cwd             string
	dryRun, noLogin bool
	timeout         time.Duration
}

// run checks the inputs and runs the step; it returns the result, the
// repository's top level and an error for a refusal or a failure before
// the images.
func (d dockerRun) run(ctx context.Context) (pipeline.DockerResult, string, error) {
	res := pipeline.DockerResult{Step: "docker", DryRun: d.dryRun, Images: []pipeline.ImageResult{}}
	top, err := gitTopLevel(ctx, d.opts.Runner, d.cwd)
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
	if err := pipeline.RemoveDockerResult(top); err != nil {
		return res, top, err
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
	res.Version, res.GitTag = rel.Version, rel.GitTag
	if !rel.Published {
		res.Status = pipeline.StatusPassed
		fmt.Fprintln(d.out, "nerv pipeline docker: no new release: nothing to build")
		return res, top, nil
	}
	if rel.DryRun && !d.dryRun {
		return res, top, refuse("release.json comes from a dry run (%s was not released): run nerv pipeline release without --dry-run first, or nerv pipeline docker --dry-run", rel.Version)
	}
	var login *pipeline.Credentials
	if !d.dryRun && !d.noLogin {
		if login, err = d.credentials(cfg.Registry); err != nil {
			return res, top, err
		}
	}

	open := d.opts.OpenDockerWorkspace
	if open == nil {
		open = func(ctx context.Context, o semrel.WorkspaceOptions) (dockerWorkspace, error) {
			return semrel.NewWorkspace(ctx, o)
		}
	}
	fmt.Fprintf(d.out, "nerv pipeline docker: reading origin's %s into a build clone\n", rel.Branch)
	ws, err := open(ctx, semrel.WorkspaceOptions{RepoRoot: top, Branch: rel.Branch, FetchTimeout: d.timeout})
	if errors.Is(err, semrel.ErrNoOrigin) {
		return res, top, refuse("the repository has no origin remote")
	}
	if err != nil {
		return res, top, err
	}
	defer ws.Close()
	revision := ws.OriginHead()
	if rel.DryRun {
		fmt.Fprintf(d.out, "nerv pipeline docker: dry-run release: building origin's %s head %s (no tag exists)\n", rel.Branch, short(revision))
	} else {
		commit, err := ws.CheckoutTag(ctx, rel.GitTag)
		if errors.Is(err, semrel.ErrTagNotOnOrigin) {
			return res, top, refuse("tag %s is not on origin: release.json does not match origin", rel.GitTag)
		}
		if err != nil {
			return res, top, err
		}
		if commit != rel.GitHead {
			return res, top, refuse("tag %s on origin points to %s, but release.json says %s: run nerv pipeline release again only if no release was made; otherwise fix release.json", rel.GitTag, short(commit), short(rel.GitHead))
		}
		revision = commit
	}
	dp := d.opts.DockerProc
	if dp == nil {
		dp = proc.ExecRunner{}
	}
	argRunner := d.opts.DockerArgs
	if argRunner == nil {
		// Computed-argument scripts do not get the pipeline's secrets either.
		argRunner = func(root string, out io.Writer) pipeline.ArgRunner {
			return &shiprun.Runner{RepoRoot: root, Output: out, Environ: func() []string { return pipeline.SafeEnv(os.Environ(), cfg.Registry) }}
		}
	}
	run := pipeline.Docker{Dir: ws.Dir(), Config: cfg, Release: rel, Revision: revision, Source: sourceURL(ws.PublicURL()),
		Proc: dp, Args: argRunner(ws.Dir(), d.out), Output: d.out, Login: login, DryRun: d.dryRun}
	return run.Run(ctx), top, nil
}

// credentials reads the registry user and token from the variables the
// registry names; both are required (--no-login relies on docker's own
// credentials instead).
func (d dockerRun) credentials(r pipelineconfig.Registry) (*pipeline.Credentials, error) {
	getenv := d.opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	user, token := getenv(r.UsernameEnv), getenv(r.TokenEnv)
	var missing []string
	if user == "" {
		missing = append(missing, r.UsernameEnv)
	}
	if token == "" {
		missing = append(missing, r.TokenEnv)
	}
	if len(missing) > 0 {
		return nil, refuse("the registry login needs %s (pipeline.registry): set them, or pass --no-login to use docker's own credentials for %s", strings.Join(missing, " and "), r.Host)
	}
	return &pipeline.Credentials{Host: r.Host, User: user, Token: token}, nil
}

// sourceURL is origin's public address as an https URL for the source
// label (git@host:owner/repo.git -> https://host/owner/repo).
func sourceURL(public string) string {
	link := semrel.ReleaseURL(public, "x")
	return strings.TrimSuffix(link, "/releases/tag/x")
}

// writeDockerResult writes the result: JSON, or a short text per image.
func writeDockerResult(w io.Writer, asJSON bool, r pipeline.DockerResult) error {
	var b bytes.Buffer
	if asJSON {
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
	} else {
		fmt.Fprintf(&b, "docker: %s\n", r.Status)
		if r.Status == pipeline.StatusPassed && len(r.Images) == 0 {
			b.WriteString("  no new release: nothing built\n")
		}
		for _, img := range r.Images {
			fmt.Fprintf(&b, "  %s %s: %s", img.Name, strings.Join(img.Tags, ","), img.Status)
			if img.Digest != "" {
				fmt.Fprintf(&b, " (%s)", img.Digest)
			}
			if img.Error != "" {
				fmt.Fprintf(&b, ": %s", img.Error)
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
