package shipplan

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// StepRunner runs one process step; *shiprun.Runner is one.
type StepRunner interface {
	Run(ctx context.Context, step shiprun.Step, vars shiprun.Vars) error
}

// Executor runs a plan's stages.
type Executor struct {
	// Steps runs the command and Exec steps.
	Steps StepRunner
	// Output receives the summary line of in-process checks, prefixed
	// "[<project>:<step>] " like shiprun's output; nil discards it.
	Output io.Writer
	// LookupEnv reads the steps' Secrets; nil means os.LookupEnv.
	LookupEnv func(string) (string, bool)
}

// StepFailure is the first step of a run that failed.
type StepFailure struct {
	Stage string
	Step  string
	Err   error
}

func (e *StepFailure) Error() string {
	return fmt.Sprintf("stage %s failed at step %s: %v", e.Stage, e.Step, e.Err)
}

func (e *StepFailure) Unwrap() error { return e.Err }

// Run runs the plan's pipeline (every stage but publish) in order and stops
// at the first failing step, returning a *StepFailure. Before anything
// runs it checks that every secret the plan will need is set: the
// publish stage's too (so a missing key fails before the build), except
// in a dry run, which never publishes. Then it empties the plan's
// ResetDirs, so a report or package of an earlier run is never used.
func (e Executor) Run(ctx context.Context, plan *Plan, vars shiprun.Vars) error {
	if err := e.checkSecrets(plan); err != nil {
		return err
	}
	if err := resetDirs(plan); err != nil {
		return err
	}
	return e.run(ctx, plan.Pipeline(), vars)
}

// resetDirs removes and recreates each of the plan's ResetDirs, after
// checking every one is strictly inside the artifacts directory (lexically:
// the plan builds them there, and a hand-built plan must not reach out).
func resetDirs(plan *Plan) error {
	root := plan.Context.ArtifactsDir
	for _, dir := range plan.ResetDirs {
		rel, err := filepath.Rel(root, dir)
		if root == "" || err != nil || rel == "." || !filepath.IsLocal(rel) {
			return fmt.Errorf("artifact directory %s is not inside the artifacts directory %s", dir, root)
		}
	}
	for _, dir := range plan.ResetDirs {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("emptying %s: %w", dir, err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// RunPublish runs the publish stage (T6 calls it inside semantic-release's
// publish step). In a dry run it runs nothing and returns nil: the stage is
// only listed (Render).
func (e Executor) RunPublish(ctx context.Context, plan *Plan, vars shiprun.Vars) error {
	pub := plan.Publish()
	if plan.Context.DryRun || pub == nil {
		return nil
	}
	if err := e.checkSecrets(plan); err != nil {
		return err
	}
	return e.run(ctx, []Stage{*pub}, vars)
}

// checkSecrets fails when a secret of a step that may run is not set or
// empty, naming the step and the variable (never a value).
func (e Executor) checkSecrets(plan *Plan) error {
	stages := plan.Stages
	if plan.Context.DryRun {
		stages = plan.Pipeline()
	}
	for _, st := range stages {
		for _, s := range st.Steps {
			for _, name := range s.Secrets {
				if v, ok := e.lookupEnv(name); !ok || v == "" {
					return fmt.Errorf("step %s: environment variable %s is not set (it holds the secret the step needs)", s.Name, name)
				}
			}
		}
	}
	return nil
}

func (e Executor) run(ctx context.Context, stages []Stage, vars shiprun.Vars) error {
	for _, st := range stages {
		for _, s := range st.Steps {
			if err := e.step(ctx, s, vars); err != nil {
				return &StepFailure{Stage: st.Name, Step: s.Name, Err: err}
			}
		}
	}
	return nil
}

func (e Executor) step(ctx context.Context, s Step, vars shiprun.Vars) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Func != nil {
		summary, err := s.Func(ctx, e.lookupEnv)
		if err != nil {
			return err
		}
		if e.Output != nil && summary != "" {
			fmt.Fprintf(e.Output, "[%s:%s] %s\n", vars.Project, s.Name, summary)
		}
		return nil
	}
	rs := shiprun.Step{Name: s.Name, Stage: s.Stage, Command: s.Command, Exec: s.Exec}
	return e.Steps.Run(ctx, rs, vars)
}

func (e Executor) lookupEnv(name string) (string, bool) {
	if e.LookupEnv != nil {
		return e.LookupEnv(name)
	}
	return os.LookupEnv(name)
}
