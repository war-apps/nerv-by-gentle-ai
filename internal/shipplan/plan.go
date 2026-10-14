package shipplan

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// ArtifactsEnv is the environment variable every step of a plan receives:
// the project's directory for build outputs, <Context.ArtifactsDir>/<name>.
// The dotnet preset writes its coverage reports to its coverage/ and its
// packages to its nuget/ subdirectory; a stage override that replaces
// those commands writes there too.
const ArtifactsEnv = "NERV_RELEASE_ARTIFACTS_DIR"

// PublishStage is the stage whose steps T6 runs inside semantic-release's
// publish step; Pipeline leaves it out and Publish returns it.
const PublishStage = "publish"

// Context is the release a plan is made for.
type Context struct {
	// Version is the next version, e.g. 1.4.0-alpha.3. Required.
	Version string
	// Channel is the release channel; empty (or "default") is the default
	// channel.
	Channel string
	// Tag is the next git tag, e.g. api-v1.4.0.
	Tag string
	// GitHead is the commit released; it labels docker images when set.
	GitHead string
	// NotesFile is the release notes file (informational for the plan).
	NotesFile string
	// DryRun marks a plan whose publish stage is listed but never run.
	DryRun bool
	// ArtifactsDir is an absolute directory, outside the repository, for
	// build outputs (coverage reports, packages). Required.
	ArtifactsDir string
}

// defaultChannel reports whether the release is on the default channel,
// as dim.comprobantes.api's workflow reads semantic-release's channel
// output: empty, or the literal "default".
func (c Context) defaultChannel() bool { return c.Channel == "" || c.Channel == "default" }

// Plan is the pipeline of one project for one release: data that can be
// rendered (Render) and run (Executor).
type Plan struct {
	Project string
	Context Context
	// Stages are the stages with commands, in shipconfig.Stages order; the
	// publish stage, when present, is last.
	Stages []Stage
	// Hooks are the commands of the semantic-release lifecycle hook keys
	// (prepare, publish, success, fail), for T6; Executor never runs them.
	Hooks map[string][]Step
}

// Stage is one pipeline stage: its before_<stage> hooks, its commands
// (a stage override, or every enabled preset's commands in preset order),
// and its after_<stage> hooks.
type Stage struct {
	Name  string
	Steps []Step
}

// Step is one thing to run. Exactly one of Check, Exec or Command.Run is
// what runs; Command also carries Env, Workdir and Timeout for Exec.
type Step struct {
	// Name is the output prefix: a preset step (dotnet:build, docker:push
	// <image>:<tag>), the stage of an override (build) or a hook key.
	Name string
	// Stage is NERV_RELEASE_STAGE: the stage, or the hook key of a hook.
	Stage   string
	Command shipconfig.Command
	// Exec is a preset command run without a shell.
	Exec []Arg
	// Check is an in-process step (the coverage check); it returns a
	// one-line summary.
	Check func(ctx context.Context) (string, error)
	// Describe says what Check does, for Render.
	Describe string
}

// Arg is one argument of an Exec step.
type Arg struct {
	Value string
	// SecretEnv, when set, names the environment variable the argument's
	// value is read from when the step runs. Render, step names and errors
	// only ever show $<SecretEnv>.
	SecretEnv string
}

// Pipeline returns every stage but publish, in order.
func (p *Plan) Pipeline() []Stage {
	var out []Stage
	for _, s := range p.Stages {
		if s.Name != PublishStage {
			out = append(out, s)
		}
	}
	return out
}

// Publish returns the publish stage, or nil when there is nothing to
// publish.
func (p *Plan) Publish() *Stage {
	for i := range p.Stages {
		if p.Stages[i].Name == PublishStage {
			return &p.Stages[i]
		}
	}
	return nil
}

// Build plans project for the release c, in the repository at repoRoot
// (read only for the node preset's package.json). The project is expected
// as shipconfig.Load returns it (defaults applied, paths normalised).
//
// Rules: a stage's commands are its override in stages:, which replaces
// every preset's commands for that stage, or else the commands of each
// enabled preset in the order of presets:. A stage with no command is
// skipped together with its before_/after_ hooks. Hooks of the lifecycle
// keys are returned in Plan.Hooks.
func Build(repoRoot string, project shipconfig.Project, c Context) (*Plan, error) {
	if c.Version == "" {
		return nil, errors.New("plan: the release version is required")
	}
	if c.ArtifactsDir == "" || !filepath.IsAbs(c.ArtifactsDir) {
		return nil, fmt.Errorf("plan: the artifacts directory must be an absolute path, got %q", c.ArtifactsDir)
	}
	b := &builder{root: repoRoot, project: project, ctx: c, artifacts: filepath.Join(c.ArtifactsDir, project.Name), steps: map[string][]Step{}}
	for _, name := range project.Presets {
		var err error
		switch name {
		case "dotnet":
			err = b.dotnet()
		case "node":
			err = b.node()
		case "docker":
			err = b.docker()
		case "nuget":
			err = b.nuget()
		case "npm":
			b.npm()
		default:
			err = fmt.Errorf("unknown preset %q", name)
		}
		if err != nil {
			return nil, fmt.Errorf("project %q: %w", project.Name, err)
		}
	}

	plan := &Plan{Project: project.Name, Context: c, Hooks: map[string][]Step{}}
	for _, stage := range shipconfig.Stages {
		cmds := b.steps[stage]
		if override, ok := project.Stages[stage]; ok {
			cmds = []Step{{Name: stage, Stage: stage, Command: override}}
		}
		if len(cmds) == 0 {
			continue
		}
		steps := b.hooks("before_" + stage)
		steps = append(steps, cmds...)
		steps = append(steps, b.hooks("after_"+stage)...)
		plan.Stages = append(plan.Stages, Stage{Name: stage, Steps: b.withArtifacts(steps)})
	}
	for _, key := range shipconfig.LifecycleHooks {
		if hooks := b.hooks(key); len(hooks) > 0 {
			plan.Hooks[key] = b.withArtifacts(hooks)
		}
	}
	return plan, nil
}

// builder collects the preset steps of one project, per stage.
type builder struct {
	root      string
	project   shipconfig.Project
	ctx       Context
	artifacts string
	steps     map[string][]Step
}

func (b *builder) add(stage string, s Step) {
	s.Stage = stage
	b.steps[stage] = append(b.steps[stage], s)
}

// hooks returns the hook commands of key as steps.
func (b *builder) hooks(key string) []Step {
	var out []Step
	for _, c := range b.project.Hooks[key] {
		out = append(out, Step{Name: key, Stage: key, Command: c})
	}
	return out
}

// withArtifacts gives every step the ArtifactsEnv variable, on a copy of
// its env map.
func (b *builder) withArtifacts(steps []Step) []Step {
	for i := range steps {
		if steps[i].Check != nil {
			continue
		}
		env := maps.Clone(steps[i].Command.Env)
		if env == nil {
			env = map[string]string{}
		}
		env[ArtifactsEnv] = b.artifacts
		steps[i].Command.Env = env
	}
	return steps
}

// enabled reports whether the project enables preset.
func (b *builder) enabled(preset string) bool { return slices.Contains(b.project.Presets, preset) }

// args turns plain strings into Exec arguments.
func args(values ...string) []Arg {
	out := make([]Arg, len(values))
	for i, v := range values {
		out[i] = Arg{Value: v}
	}
	return out
}
