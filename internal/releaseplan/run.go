package releaseplan

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaserun"
)

// StepRunner runs one process step; *releaserun.Runner is one.
type StepRunner interface {
	Run(ctx context.Context, step releaserun.Step, vars releaserun.Vars) error
}

// Executor runs a plan's stages.
type Executor struct {
	// Steps runs the command and Exec steps.
	Steps StepRunner
	// Output receives the summary line of in-process checks, prefixed
	// "[<project>:<step>] " like releaserun's output; nil discards it.
	Output io.Writer
}

// StepFailure is the first step of a run that failed.
type StepFailure struct {
	Stage string
	Step  string
	// Index is the step's position in its stage's Steps (or in the hook
	// key's list): the steps before it succeeded.
	Index int
	Err   error
}

func (e *StepFailure) Error() string {
	return fmt.Sprintf("stage %s failed at step %s: %v", e.Stage, e.Step, e.Err)
}

func (e *StepFailure) Unwrap() error { return e.Err }

// Run runs the plan's stages in order and stops at the first failing step,
// returning a *StepFailure. Before anything runs it empties the plan's
// ResetDirs, so a report or package of an earlier run is never used.
func (e Executor) Run(ctx context.Context, plan *Plan, vars releaserun.Vars) error {
	if err := resetDirs(plan); err != nil {
		return err
	}
	return e.run(ctx, plan.Stages, vars)
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

// RunHooks runs the plan's hooks of one lifecycle key (prepare, fail) in
// order and stops at the first failure, a *StepFailure
// whose Stage is the key. No hook for the key is no error. It runs in a dry
// run too: the caller decides which keys a dry run reaches.
func (e Executor) RunHooks(ctx context.Context, plan *Plan, key string, vars releaserun.Vars) error {
	hooks := plan.Hooks[key]
	if len(hooks) == 0 {
		return nil
	}
	return e.run(ctx, []Stage{{Name: key, Steps: hooks}}, vars)
}

func (e Executor) run(ctx context.Context, stages []Stage, vars releaserun.Vars) error {
	probe := NewBuildServersProbe(e.Steps)
	for _, st := range stages {
		for i, s := range st.Steps {
			if err := e.step(ctx, s, vars, probe); err != nil {
				return &StepFailure{Stage: st.Name, Step: s.Name, Index: i, Err: err}
			}
		}
	}
	return nil
}

func (e Executor) step(ctx context.Context, s Step, vars releaserun.Vars, probe *BuildServersProbe) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Func != nil {
		summary, err := s.Func(ctx)
		if err != nil {
			return err
		}
		if e.Output != nil && summary != "" {
			fmt.Fprintf(e.Output, "[%s:%s] %s\n", vars.Project, s.Name, summary)
		}
		return nil
	}
	rs := releaserun.Step{Name: s.Name, Stage: s.Stage, Command: s.Command, Exec: s.Exec}
	var note string
	rs.Exec, note = probe.Argv(ctx, rs, vars)
	if e.Output != nil && note != "" {
		fmt.Fprintf(e.Output, "[%s:%s] %s\n", vars.Project, s.Name, note)
	}
	return e.Steps.Run(ctx, rs, vars)
}
