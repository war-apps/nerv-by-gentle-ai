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
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deploy"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

var errDeployDeclined = errors.New("cancelled: nothing was deployed")

// runPipelineDeploy is `nerv pipeline deploy`: dim's /update-deploy for
// the release just made, from the pipeline.deploy data (environment
// variables or Infisical), over SSH.
func runPipelineDeploy(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("pipeline deploy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	d := deployRun{opts: opts}
	cwd := fs.String("cwd", ".", "")
	fs.BoolVar(&d.dryRun, "dry-run", false, "")
	fs.BoolVar(&d.yes, "yes", false, "")
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
	d.cwd = *cwd
	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop, received := signalCtx(context.Background())
	defer stop()

	out := pipeline.NewPrefixWriter(stderr, "[pipeline:deploy] ")
	defer out.Flush()
	d.out = out
	res, top, err := d.run(ctx)
	// deploy.json records every run that reached its outcome (deployed,
	// nothing to deploy, a dry run, a failed or interrupted deploy); a
	// refusal or a failure before it never touches an earlier one.
	code, write := 0, false
	switch {
	case ctx.Err() != nil:
		res.Status, code = pipeline.StatusInterrupted, interruptedExitCode(received())
		write = err == nil && len(res.Environments) > 0
		fmt.Fprintln(out, "nerv pipeline deploy: interrupted")
	case err != nil:
		res.Status, res.Error, code = pipeline.StatusFailed, err.Error(), 2
		if refusal.Is(err) {
			res.Status, code = pipelineRefused, 1
		}
		fmt.Fprintf(out, "nerv pipeline deploy: %v\n", err)
	case res.Status != pipeline.StatusPassed:
		code, write = 2, true
		fmt.Fprintf(out, "nerv pipeline deploy: %s\n", res.Error)
		if res.Action != "" {
			fmt.Fprintf(out, "nerv pipeline deploy: %s\n", res.Action)
		}
	default:
		write = true
	}
	if write {
		if path, werr := pipeline.WriteDeployResult(top, res); werr != nil {
			res.Error = "writing " + pipeline.DeployResultFile + ": " + werr.Error()
			if code == 0 {
				res.Status, code = pipeline.StatusFailed, 2
			}
			fmt.Fprintf(out, "nerv pipeline deploy: %s\n", res.Error)
		} else {
			fmt.Fprintf(out, "nerv pipeline deploy: result in %s\n", path)
		}
	}
	out.Flush()
	if werr := writeDeployResult(stdout, *jsonOut, res); werr != nil && code == 0 {
		code = 2
	}
	return code
}

type deployRun struct {
	opts        options
	out         io.Writer
	cwd         string
	dryRun, yes bool
}

func (d deployRun) run(ctx context.Context) (pipeline.DeployResult, string, error) {
	res := pipeline.DeployResult{Step: "deploy", DryRun: d.dryRun, Environments: []pipeline.EnvironmentDeploy{}}
	top, err := gitTopLevel(ctx, d.opts.Runner, d.cwd)
	if err != nil {
		return res, "", err
	}
	cfg, err := pipelineconfig.Load(top)
	switch {
	case errors.Is(err, pipelineconfig.ErrNotConfigured):
		return res, top, refuse("%s", withLegacyHint("the repository has no pipeline: section in .ai-workflow.yml; see docs/pipeline.md", top))
	case err != nil:
		return res, top, &refusal.Error{Err: err}
	}
	if cfg.Deploy == nil {
		res.Status = pipeline.StatusPassed
		fmt.Fprintln(d.out, "nerv pipeline deploy: pipeline.deploy is not configured: nothing to do")
		return res, top, nil
	}
	dc := cfg.Deploy
	res.Source, res.Selection = dc.Source, dc.Selection
	rel, err := pipeline.ReadReleaseOutputs(top)
	switch {
	case errors.Is(err, pipeline.ErrNoReleaseOutputs):
		return res, top, refuse("no release outputs (%s): run nerv pipeline release first", pipeline.ReleaseOutputsPath(top))
	case err != nil:
		return res, top, err
	case rel.Status != pipeline.StatusPassed:
		return res, top, refuse("the last release step did not pass (status %s): run nerv pipeline release first", rel.Status)
	}
	res.Version, res.GitTag, res.Branch = rel.Version, rel.GitTag, rel.Branch
	if !rel.Published {
		res.Status = pipeline.StatusPassed
		fmt.Fprintln(d.out, "nerv pipeline deploy: no new release: nothing to deploy")
		return res, top, nil
	}
	if rel.DryRun && !d.dryRun {
		return res, top, refuse("release.json comes from a dry run (%s was not released): run nerv pipeline release without --dry-run first, or nerv pipeline deploy --dry-run", rel.Version)
	}
	// Deploy only the image the docker step pushed for this release.
	if err := pipeline.CheckDeployEvidence(top, rel); err != nil {
		if errors.Is(err, pipeline.ErrMissingEvidence) {
			return res, top, &refusal.Error{Err: err}
		}
		return res, top, err
	}

	runner := d.opts.DeployProc
	if runner == nil {
		runner = proc.ExecRunner{}
	}
	// Children never get the registry secrets or the NERV_DEPLOY_* data;
	// ssh gets SSHPASS alone with auth password.
	childEnv := pipeline.SafeEnv(os.Environ(), cfg.Registry)
	var src deploy.Source = deploy.EnvSource{Getenv: d.getenv()}
	if dc.Source == "infisical" {
		src = deploy.InfisicalSource{Proc: runner, ProjectID: dc.Infisical.ProjectID, AppPath: dc.Infisical.AppPath, Domain: dc.Infisical.Domain, Env: childEnv}
	}
	fmt.Fprintf(d.out, "nerv pipeline deploy: %s %s (selection %s, data from %s)\n", rel.Version, rel.Branch, dc.Selection, dc.Source)
	plan, err := deploy.Plan(ctx, src, dc, rel)
	if errors.Is(err, deploy.ErrIncompleteData) {
		return res, top, &refusal.Error{Err: err}
	}
	if err != nil {
		return res, top, err
	}
	deploying := 0
	for _, p := range plan {
		res.Environments = append(res.Environments, p.Result)
		if !p.Deploy {
			fmt.Fprintf(d.out, "nerv pipeline deploy: %s: skipped: %s\n", p.Result.Environment, p.Result.Reason)
			continue
		}
		deploying++
		fmt.Fprintf(d.out, "nerv pipeline deploy: %s: deploy %s on %s with %s\n", p.Result.Environment, p.Target.Container, p.Target.Host, p.Target.ComposeScript)
		if p.Target.Strategy == "blue-green" {
			fmt.Fprintf(d.out, "nerv pipeline deploy: warning: %s is blue-green: if it is the live container, the update recreates it and it is down until healthy (about 30s); the usual flow updates the idle one and then switches the proxy\n", p.Target.Container)
		}
	}
	if deploying == 0 {
		res.Status = pipeline.StatusPassed
		fmt.Fprintln(d.out, "nerv pipeline deploy: no environment to deploy for this release")
		return res, top, nil
	}
	if d.dryRun {
		res.Status = pipeline.StatusPassed
		fmt.Fprintln(d.out, "nerv pipeline deploy: dry run: no SSH connection made")
		return res, top, nil
	}
	if err := d.confirm(deploying); err != nil {
		return res, top, err
	}
	home, err := configure.ResolveHome(d.opts.Home)
	if err != nil {
		return res, top, err
	}
	ssh := deploy.SSH{Proc: runner, Auth: dc.SSH.Auth, KeyFile: expandHome(dc.SSH.KeyFile, home), KnownHosts: expandHome(dc.SSH.KnownHosts, home),
		Port: dc.SSH.Port, ConnectTimeout: dc.SSH.ConnectTimeoutDuration(), Env: childEnv}
	r := deploy.Runner{SSH: ssh, Verify: dc.Verify, Release: rel, Output: d.out, Sleep: d.opts.DeploySleep, UpdateTimeout: dc.UpdateTimeoutDuration()}
	envs, failed := r.Run(ctx, plan)
	res.Environments = envs
	res.Status = pipeline.StatusPassed
	if failed != "" {
		res.Status = pipeline.StatusFailed
		res.Error = "deploying " + failed + " failed"
		res.Action = "no rollback was made and the environments after it were not deployed: fix " + failed + " and run nerv pipeline deploy again"
	}
	return res, top, nil
}

func (d deployRun) getenv() func(string) string {
	if d.opts.Getenv != nil {
		return d.opts.Getenv
	}
	return os.Getenv
}

// confirm asks before deploying on an interactive terminal; --yes skips it,
// and without a terminal --yes is required.
func (d deployRun) confirm(n int) error {
	if d.yes {
		return nil
	}
	if d.opts.IsTerminal == nil || !d.opts.IsTerminal() || d.opts.Stdin == nil {
		return refuse("confirmation needed but stdin is not an interactive terminal: pass --yes to deploy without asking (or --dry-run to preview)")
	}
	fmt.Fprintf(d.out, "Deploy %d environment(s)? [y/N]: ", n)
	line, sawNewline := readAnswer(d.opts.Stdin)
	if !sawNewline {
		fmt.Fprintln(d.out)
	}
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		return &refusal.Error{Err: errDeployDeclined}
	}
	return nil
}

// expandHome expands a leading ~/ to home.
func expandHome(p, home string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, filepath.FromSlash(rest))
	}
	return p
}

// writeDeployResult writes the result: JSON, or a short text per
// environment.
func writeDeployResult(w io.Writer, asJSON bool, r pipeline.DeployResult) error {
	var b bytes.Buffer
	if asJSON {
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
	} else {
		fmt.Fprintf(&b, "deploy: %s\n", r.Status)
		for _, e := range r.Environments {
			fmt.Fprintf(&b, "  %s: %s", e.Environment, e.Status)
			switch {
			case e.Running != "":
				fmt.Fprintf(&b, " (%s %s)", e.Container, e.Running)
			case e.Reason != "":
				fmt.Fprintf(&b, ": %s", e.Reason)
			}
			if e.Error != "" {
				fmt.Fprintf(&b, ": %s", e.Error)
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
