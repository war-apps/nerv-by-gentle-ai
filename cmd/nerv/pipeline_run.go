package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// runStepFlags are the flags nerv pipeline run passes to each step besides
// --cwd and --json: validate and test take neither --dry-run nor --yes
// (they only check the working tree); docker takes --dry-run but asks
// nothing (its pushes follow the release the user confirmed).
var runStepFlags = map[string]struct{ dryRun, yes bool }{
	"validate": {}, "test": {},
	"release": {true, true}, "docker": {true, false}, "sync": {true, true}, "deploy": {true, true},
}

// runPipelineRun is `nerv pipeline run`: every step in dim's order, each
// through its own implementation (its records, gates and confirmations),
// stopping at the first step that does not pass.
func runPipelineRun(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("pipeline run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cwd := fs.String("cwd", ".", "")
	dryRun := fs.Bool("dry-run", false, "")
	yes := fs.Bool("yes", false, "")
	jsonOut := fs.Bool("json", false, "")
	from := fs.String("from", "", "")
	only := fs.String("only", "", "")
	var expect expectList
	fs.Var(&expect, "expect", "")
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
	steps, err := runSteps(*from, *only)
	if err != nil {
		return pipelineUsageError(stderr, err.Error())
	}
	out := pipeline.NewPrefixWriter(stderr, "[pipeline:run] ")
	defer out.Flush()
	res := pipeline.RunResult{Step: "run", DryRun: *dryRun, Steps: []pipeline.StepOutcome{}}
	top, err := gitTopLevel(context.Background(), opts.Runner, *cwd)
	if err != nil {
		res.Status, res.Error = pipeline.StatusFailed, err.Error()
		code := 2
		if refusal.Is(err) {
			res.Status, code = pipelineRefused, 1
		}
		fmt.Fprintf(out, "nerv pipeline run: %v\n", err)
		out.Flush()
		writeRunResult(stdout, *jsonOut, res)
		return code
	}
	// An earlier run.json must never pass for this run's result.
	if err := pipeline.RemoveRunResult(top); err != nil {
		res.Status, res.Error = pipeline.StatusFailed, err.Error()
		fmt.Fprintf(out, "nerv pipeline run: %v\n", err)
		out.Flush()
		writeRunResult(stdout, *jsonOut, res)
		return 2
	}
	step := opts.PipelineStep
	if step == nil {
		step = func(name string, args []string, stdout, stderr io.Writer, opts options) int {
			return runPipeline(append([]string{name}, args...), stdout, stderr, opts)
		}
	}
	code := 0
	res.Status = pipeline.StatusPassed
	for _, name := range steps {
		if code != 0 {
			res.Steps = append(res.Steps, pipeline.StepOutcome{Step: name, Status: pipeline.StepNotRun})
			continue
		}
		stepArgs := []string{"--cwd", top, "--json"}
		if f := runStepFlags[name]; f.dryRun && *dryRun {
			stepArgs = append(stepArgs, "--dry-run")
		} else if f.yes && *yes {
			stepArgs = append(stepArgs, "--yes")
		}
		if name == "release" {
			// The release step refuses anything but the approved versions.
			for _, e := range expect {
				stepArgs = append(stepArgs, "--expect", e)
			}
		}
		fmt.Fprintf(out, "%s\n", name)
		out.Flush()
		var doc bytes.Buffer
		exit := step(name, stepArgs, &doc, stderr, opts)
		o := pipeline.StepOutcome{Step: name, Exit: exit}
		var parsed struct{ Status, Error string }
		if err := json.Unmarshal(lastJSONLine(doc.Bytes()), &parsed); err == nil && parsed.Status != "" {
			o.Status, o.Error = parsed.Status, parsed.Error
		} else {
			o.Status, o.Error = pipeline.StatusFailed, fmt.Sprintf("exited %d without a result document", exit)
		}
		res.Steps = append(res.Steps, o)
		// Both must agree: exit 0 and an accepted status. A step whose
		// document disagrees with its exit stops the run too.
		if exit != 0 || !slices.Contains(pipeline.RunAccepted, o.Status) {
			if o.Error == "" {
				o.Error = fmt.Sprintf("exited %d with status %s", exit, o.Status)
				res.Steps[len(res.Steps)-1].Error = o.Error
			}
			code = exit
			if code == 0 {
				code = 2
			}
			res.Status = o.Status
			res.Error = name + ": " + o.Error
			fmt.Fprintf(out, "%s %s (exit %d): %s; the later steps were not run\n", name, o.Status, exit, o.Error)
		}
	}
	if code == 0 {
		fmt.Fprintf(out, "every step passed\n")
	}
	if path, err := pipeline.WriteRunResult(top, res); err != nil {
		fmt.Fprintf(out, "nerv pipeline run: writing %s: %v\n", pipeline.RunResultFile, err)
		if code == 0 {
			code = 2
		}
	} else {
		fmt.Fprintf(out, "result in %s\n", path)
	}
	out.Flush()
	if err := writeRunResult(stdout, *jsonOut, res); err != nil && code == 0 {
		code = 2
	}
	return code
}

// runSteps returns the steps to run: every step, from --from on, or
// --only's one.
func runSteps(from, only string) ([]string, error) {
	switch {
	case from != "" && only != "":
		return nil, errors.New("--from and --only cannot be combined")
	case only != "":
		if !slices.Contains(pipelineStepNames, only) {
			return nil, fmt.Errorf("--only %q: unknown step (want one of %s)", only, strings.Join(pipelineStepNames, ", "))
		}
		return []string{only}, nil
	case from != "":
		i := slices.Index(pipelineStepNames, from)
		if i < 0 {
			return nil, fmt.Errorf("--from %q: unknown step (want one of %s)", from, strings.Join(pipelineStepNames, ", "))
		}
		return pipelineStepNames[i:], nil
	}
	return pipelineStepNames, nil
}

// lastJSONLine is the last non-empty line of a step's stdout (its JSON
// document).
func lastJSONLine(b []byte) []byte {
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	return lines[len(lines)-1]
}

// writeRunResult writes the run document: JSON, or one line per step.
func writeRunResult(w io.Writer, asJSON bool, r pipeline.RunResult) error {
	var b bytes.Buffer
	if asJSON {
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
	} else {
		fmt.Fprintf(&b, "run: %s\n", r.Status)
		for _, s := range r.Steps {
			fmt.Fprintf(&b, "  %s: %s", s.Step, s.Status)
			if s.Exit != 0 {
				fmt.Fprintf(&b, " (exit %d)", s.Exit)
			}
			if s.Error != "" {
				fmt.Fprintf(&b, ": %s", s.Error)
			}
			b.WriteByte('\n')
		}
	}
	_, err := w.Write(b.Bytes())
	return err
}

// expectList collects repeated --expect values for the release step.
type expectList []string

func (e *expectList) String() string     { return strings.Join(*e, ",") }
func (e *expectList) Set(v string) error { *e = append(*e, v); return nil }
