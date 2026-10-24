package shipplan

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// The environment variables every step of a plan receives, besides
// shiprun's NERV_RELEASE_* release variables.
const (
	// ArtifactsEnv is the project's directory for build outputs, outside
	// the release clone: <Context.ArtifactsDir>/<name>.
	ArtifactsEnv = "NERV_RELEASE_ARTIFACTS_DIR"
	// OutputsEnv is where the package stage writes what goes into the
	// artifact's outputs/: <ArtifactsEnv>/outputs, emptied before the run.
	OutputsEnv = "NERV_RELEASE_OUTPUTS_DIR"
	// CoverageEnv is the directory the coverage check searches for
	// Cobertura reports: build.coverage.dir in the clone, or
	// <ArtifactsEnv>/coverage (emptied before the run). Set only when
	// build.coverage is configured.
	CoverageEnv = "NERV_RELEASE_COVERAGE_DIR"
)

// Context is the release a plan is made for.
type Context struct {
	// Version is the next version, e.g. 1.4.0-alpha.3. Required.
	Version string
	// Channel is the release channel; empty (or "default") is the default
	// channel.
	Channel string
	// Tag is the next git tag, e.g. api-v1.4.0.
	Tag string
	// GitHead is the commit built.
	GitHead string
	// DryRun marks a rehearsal: the pipeline and hooks run, nothing is
	// packaged into an artifact.
	DryRun bool
	// ArtifactsDir is an absolute directory, outside the repository, for
	// build outputs (coverage reports, packages). Required.
	ArtifactsDir string
}

// defaultChannel reports whether the release is on the default channel:
// empty, or the literal "default".
func (c Context) defaultChannel() bool { return c.Channel == "" || c.Channel == "default" }

// Plan is the pipeline of one project for one release: data that can be
// rendered (Render) and run (Executor).
type Plan struct {
	Project string
	// Mode is the build mode, shipconfig.ModeCompose or ModeScript.
	Mode    string
	Context Context
	// Stages are the stages with commands, in shipconfig.Stages order.
	Stages []Stage
	// Hooks are the commands of the lifecycle hook keys (prepare, fail);
	// Executor.Run never runs them, RunHooks does.
	Hooks map[string][]Step
	// ResetDirs are the project's directories Executor.Run empties before
	// the first stage, so outputs of an earlier run (coverage reports,
	// packages) are never counted or packaged. Each must be inside
	// Context.ArtifactsDir.
	ResetDirs []string
	// OutputsDir is the package stage's output directory (OutputsEnv):
	// what it holds after the run becomes the artifact's outputs/.
	OutputsDir string
	// Images are the images of build.images, saved after the package
	// stage's commands, each to its tar under OutputsDir.
	Images []Image
}

// Image is one image of build.images, saved into the artifact.
type Image struct {
	// Ref is the image reference, placeholders expanded.
	Ref string
	// Tar is the archive's name under Plan.OutputsDir.
	Tar string
}

// Stage is one pipeline stage: its before_<stage> hooks, its commands and
// its after_<stage> hooks.
type Stage struct {
	Name  string
	Steps []Step
}

// Step is one thing to run. Exactly one of Func, Exec or Command.Run is
// what runs; Command also carries Env, Workdir and Timeout for Exec.
type Step struct {
	// Name is the output prefix: the stage of a script command (build), a
	// built-in step (compose:build, git:submodules, coverage:check) or a
	// hook key.
	Name string
	// Stage is NERV_RELEASE_STAGE: the stage, or the hook key of a hook.
	Stage   string
	Command shipconfig.Command
	// Exec is a built-in command run without a shell.
	Exec []string
	// Func is an in-process step (the coverage check); it returns a
	// one-line summary.
	Func func(ctx context.Context) (string, error)
	// Describe says what Func does, for Render.
	Describe string
}

// sharedHostStages are the script stages that get the build-server
// environment with build.shared_host: the stages dotnet commands run in.
var sharedHostStages = []string{"restore", "lint", "build", "test", "package"}

// Build plans project for the release c, in the release clone at repoRoot.
// The project is expected as shipconfig.Load returns it (defaults applied,
// paths normalised); its build.mode is required.
//
// Rules: in script mode each stage runs the project's command, and the
// package stage ends with one docker save per image of build.images; in compose
// mode build is docker compose -f <file> build, test is docker compose -f
// <file> run --rm <test_service> (no test_service: no test stage), package
// is one docker save per image of build.images, and only the restore, lint
// and coverage stages may have the project's commands. A stage with no
// command is skipped together with its before_/after_ hooks. The restore
// stage starts with git submodule update when submodules is set, and the
// coverage stage ends with the floor check when build.coverage is set;
// both count as commands. With build.shared_host the restore, lint, build,
// test and package commands get the build-server environment under their
// own. Hooks of the lifecycle keys are returned in Plan.Hooks.
func Build(repoRoot string, project shipconfig.Project, c Context) (*Plan, error) {
	if c.Version == "" {
		return nil, errors.New("plan: the release version is required")
	}
	if c.ArtifactsDir == "" || !filepath.IsAbs(c.ArtifactsDir) {
		return nil, fmt.Errorf("plan: the artifacts directory must be an absolute path, got %q", c.ArtifactsDir)
	}
	bc := project.Build
	if bc == nil || !slices.Contains(shipconfig.Modes, bc.Mode) {
		return nil, fmt.Errorf("project %q: build.mode is required (compose or script)", project.Name)
	}
	dir := filepath.Join(c.ArtifactsDir, project.Name)
	b := &builder{root: repoRoot, project: project, build: bc, ctx: c, dir: dir, steps: map[string][]Step{}}
	plan := &Plan{Project: project.Name, Mode: bc.Mode, Context: c, Hooks: map[string][]Step{}, OutputsDir: filepath.Join(dir, "outputs")}
	b.resetDirs = []string{plan.OutputsDir}
	if bc.Mode == shipconfig.ModeCompose {
		b.compose()
	}
	for _, stage := range shipconfig.Stages {
		if cmd, ok := project.Stages[stage]; ok {
			if bc.SharedHost && slices.Contains(sharedHostStages, stage) {
				env := DotnetBuildServerEnv()
				maps.Copy(env, cmd.Env)
				cmd.Env = env
			}
			b.add(stage, Step{Name: stage, Command: cmd})
		}
	}
	// The images are saved once the package stage's commands (script mode)
	// or compose build have built them.
	images, err := b.saveImages(plan.OutputsDir)
	if err != nil {
		return nil, fmt.Errorf("project %q: %w", project.Name, err)
	}
	plan.Images = images
	b.coverage()

	for _, stage := range shipconfig.Stages {
		cmds := b.steps[stage]
		if stage == "restore" {
			cmds = append(b.setupSteps(), cmds...)
		}
		if len(cmds) == 0 {
			continue
		}
		var steps []Step
		steps = append(steps, b.hooks("before_"+stage)...)
		steps = append(steps, cmds...)
		steps = append(steps, b.hooks("after_"+stage)...)
		plan.Stages = append(plan.Stages, Stage{Name: stage, Steps: b.withDirs(steps)})
	}
	for _, key := range shipconfig.LifecycleHooks {
		if hooks := b.hooks(key); len(hooks) > 0 {
			plan.Hooks[key] = b.withDirs(hooks)
		}
	}
	plan.ResetDirs = b.resetDirs
	return plan, nil
}

// builder collects the steps of one project, per stage.
type builder struct {
	root        string
	project     shipconfig.Project
	build       *shipconfig.Build
	ctx         Context
	dir         string
	coverageDir string
	steps       map[string][]Step
	resetDirs   []string
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

// withDirs gives every process step ArtifactsEnv, OutputsEnv and, with a
// coverage check, CoverageEnv, on a copy of its env map.
func (b *builder) withDirs(steps []Step) []Step {
	for i := range steps {
		if steps[i].Func != nil {
			continue
		}
		env := maps.Clone(steps[i].Command.Env)
		if env == nil {
			env = map[string]string{}
		}
		env[ArtifactsEnv] = b.dir
		env[OutputsEnv] = filepath.Join(b.dir, "outputs")
		if b.coverageDir != "" {
			env[CoverageEnv] = b.coverageDir
		}
		steps[i].Command.Env = env
	}
	return steps
}

// setupSteps prepare the release clone before the restore stage's hooks
// and commands: the submodules.
func (b *builder) setupSteps() []Step {
	if !b.project.Submodules {
		return nil
	}
	return []Step{{Name: "git:submodules", Stage: "restore", Exec: []string{"git", "submodule", "update", "--init", "--recursive"}}}
}

// imageRefRe is an image reference once its placeholders are expanded:
// no whitespace, no shell or cmd.exe metacharacter.
var imageRefRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]*$`)

// placeholderRe matches a ${name} placeholder of an image reference.
var placeholderRe = regexp.MustCompile(`\$\{([a-z_]+)\}`)

// compose plans compose mode's build and test commands.
func (b *builder) compose() {
	file := b.build.File
	b.add("build", Step{Name: "compose:build", Exec: []string{"docker", "compose", "-f", file, "build"}})
	if svc := b.build.TestService; svc != "" {
		b.add("test", Step{Name: "compose:test", Exec: []string{"docker", "compose", "-f", file, "run", "--rm", svc}})
	}
}

// saveImages plans one docker save per image of build.images at the end of
// the package stage, and returns the images saved.
func (b *builder) saveImages(outputs string) ([]Image, error) {
	name := "docker:save "
	if b.build.Mode == shipconfig.ModeCompose {
		name = "compose:save "
	}
	var images []Image
	for _, raw := range b.build.Images {
		ref := b.expand(raw)
		if !imageRefRe.MatchString(ref) {
			return nil, fmt.Errorf("build.images: %q expands to %q, not an image reference", raw, ref)
		}
		if slices.ContainsFunc(images, func(i Image) bool { return i.Ref == ref }) {
			continue
		}
		img := Image{Ref: ref, Tar: tarName(ref)}
		images = append(images, img)
		b.add("package", Step{Name: name + ref, Exec: []string{"docker", "save", "-o", filepath.Join(outputs, img.Tar), ref}})
	}
	return images, nil
}

// tarName is the archive name of an image reference: its / : and @ as _.
func tarName(ref string) string {
	return strings.NewReplacer("/", "_", ":", "_", "@", "_").Replace(ref) + ".tar"
}

// expand replaces the placeholders ${version}, ${channel} and ${tag} (the
// channel is empty on the default channel).
func (b *builder) expand(s string) string {
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		switch placeholderRe.FindStringSubmatch(m)[1] {
		case "version":
			return b.ctx.Version
		case "channel":
			if b.ctx.defaultChannel() {
				return ""
			}
			return b.ctx.Channel
		case "tag":
			return b.ctx.Tag
		}
		return m
	})
}

// coverage plans the floor check of build.coverage at the end of the
// coverage stage.
func (b *builder) coverage() {
	c := b.build.Coverage
	if c == nil || !c.Floors().Enabled() {
		return
	}
	dir := filepath.Join(b.dir, "coverage")
	if c.Dir != "" {
		dir = filepath.Join(b.root, filepath.FromSlash(c.Dir))
	} else {
		b.resetDirs = append(b.resetDirs, dir)
	}
	b.coverageDir = dir
	floors := c.Floors()
	b.add("coverage", Step{
		Name:     "coverage:check",
		Describe: fmt.Sprintf("check %s in %s", describeFloors(floors), dir),
		Func: func(context.Context) (string, error) {
			cov, err := CheckCoverageFloors(dir, floors)
			if err != nil {
				return "", err
			}
			return cov.String(), nil
		},
	})
}

// describeFloors is the floors checked, e.g. "line coverage >= 80%,
// method coverage >= 85%".
func describeFloors(f shipconfig.CoverageThreshold) string {
	var parts []string
	for _, m := range []struct {
		name string
		v    float64
	}{{"line", f.Line}, {"branch", f.Branch}, {"method", f.Method}} {
		if m.v > 0 {
			parts = append(parts, fmt.Sprintf("%s coverage >= %s%%", m.name, strconv.FormatFloat(m.v, 'f', -1, 64)))
		}
	}
	return strings.Join(parts, ", ")
}

// Outputs lists what the package stage left in OutputsDir, sorted by name,
// as the artifact's outputs: each saved image's tar (an image-tar, which
// must be there), and every other entry as a file or a directory. A
// missing OutputsDir is no outputs.
func (p *Plan) Outputs() ([]artifact.OutputSource, error) {
	entries, err := os.ReadDir(p.OutputsDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	tars := map[string]bool{}
	for _, img := range p.Images {
		tars[img.Tar] = true
		if !slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.Name() == img.Tar }) {
			return nil, fmt.Errorf("the image %s was not saved to %s", img.Ref, filepath.Join(p.OutputsDir, img.Tar))
		}
	}
	out := []artifact.OutputSource{}
	for _, e := range entries {
		o := artifact.OutputSource{Path: e.Name(), From: filepath.Join(p.OutputsDir, e.Name())}
		switch {
		case tars[e.Name()]:
			o.Kind = artifact.KindImageTar
		case e.IsDir():
			o.Kind = artifact.KindDir
		case e.Type().IsRegular():
			o.Kind = artifact.KindFile
		default:
			return nil, fmt.Errorf("output %s is not a regular file or directory", o.From)
		}
		out = append(out, o)
	}
	return out, nil
}
