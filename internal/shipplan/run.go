package shipplan

import (
	"context"
	"fmt"
	"io"
	"os"

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
	// LookupEnv reads the secrets of Exec arguments; nil means os.LookupEnv.
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
// in a dry run, which never publishes.
func (e Executor) Run(ctx context.Context, plan *Plan, vars shiprun.Vars) error {
	if err := e.checkSecrets(plan); err != nil {
		return err
	}
	return e.run(ctx, plan.Pipeline(), vars)
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
			for _, a := range s.Exec {
				if a.SecretEnv == "" {
					continue
				}
				if v, ok := e.lookupEnv(a.SecretEnv); !ok || v == "" {
					return fmt.Errorf("step %s: environment variable %s is not set (it holds the secret the step needs)", s.Name, a.SecretEnv)
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
	if s.Check != nil {
		summary, err := s.Check(ctx)
		if err != nil {
			return err
		}
		if e.Output != nil {
			fmt.Fprintf(e.Output, "[%s:%s] %s\n", vars.Project, s.Name, summary)
		}
		return nil
	}
	rs := shiprun.Step{Name: s.Name, Stage: s.Stage, Command: s.Command}
	for _, a := range s.Exec {
		v := a.Value
		if a.SecretEnv != "" {
			v, _ = e.lookupEnv(a.SecretEnv)
		}
		rs.Exec = append(rs.Exec, v)
	}
	return e.Steps.Run(ctx, rs, vars)
}

func (e Executor) lookupEnv(name string) (string, bool) {
	if e.LookupEnv != nil {
		return e.LookupEnv(name)
	}
	return os.LookupEnv(name)
}
