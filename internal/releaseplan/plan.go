package releaseplan

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
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
)

// The environment variables every step of a plan receives, besides
// releaserun's NERV_RELEASE_* release variables.
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
	// Provider is the project's build provider (docker-compose, bash).
	Provider string
	Context  Context
	// Stages are the stages with commands, in releaseconfig.Stages order.
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
	// Images are the project's images, saved after the package
	// stage's commands, each to its tar under OutputsDir.
	Images []Image
}

// Image is one image of the project's images, saved into the artifact.
type Image struct {
	// Ref is the image's first reference (Refs[0]), placeholders expanded.
	Ref string
	// Refs are every reference saved in Tar and recorded in the manifest:
	// the one reference of a plain-string entry, or the tags of a {name,
	// tags} entry, in order, without a reference an earlier image saves.
	Refs []string
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
	// Name is the output prefix: the stage of a project command (build), a
	// built-in step (compose:build, git:submodules, coverage:check) or a
	// hook key.
	Name string
	// Stage is NERV_RELEASE_STAGE: the stage, or the hook key of a hook.
	Stage   string
	Command releaseconfig.Command
	// Exec is a built-in command run without a shell.
	Exec []string
	// Func is an in-process step (the coverage check); it returns a
	// one-line summary.
	Func func(ctx context.Context) (string, error)
	// Describe says what Func does, for Render.
	Describe string
}

// Build plans project for the release c, in the release clone at repoRoot,
// with the steps its build provider planned (BuildProvider.Plan). The
// project is expected as releaseconfig.Load returns it (defaults applied,
// paths normalised).
//
// Rules: each stage runs the provider's built-in steps (docker-compose:
// build is docker compose -f <file> build, test is docker compose -f
// <file> run --rm <test_service>), then the project's own command of the
// stage, its env under the provider's stage env (bash's shared_host: the
// build-server environment); the package stage ends with the project's
// images (saveImages: docker tag for a {name, tags} entry's other tags,
// then one docker save per image, named with the provider's image prefix).
// A stage with no command is skipped together with its before_/after_
// hooks. The restore stage starts with git submodule update when
// submodules is set, and the coverage stage ends with the floor check when
// coverage is set; both count as commands. Hooks of the lifecycle keys are
// returned in Plan.Hooks.
func Build(repoRoot string, project releaseconfig.Project, c Context, build BuildSteps) (*Plan, error) {
	if c.Version == "" {
		return nil, errors.New("plan: the release version is required")
	}
	if c.ArtifactsDir == "" || !filepath.IsAbs(c.ArtifactsDir) {
		return nil, fmt.Errorf("plan: the artifacts directory must be an absolute path, got %q", c.ArtifactsDir)
	}
	if build.Provider == "" {
		return nil, fmt.Errorf("project %q: the build provider is required", project.Name)
	}
	dir := filepath.Join(c.ArtifactsDir, project.Name)
	b := &builder{root: repoRoot, project: project, build: build, ctx: c, dir: dir, steps: map[string][]Step{}}
	plan := &Plan{Project: project.Name, Provider: build.Provider, Context: c, Hooks: map[string][]Step{}, OutputsDir: filepath.Join(dir, "outputs")}
	b.resetDirs = []string{plan.OutputsDir}
	for _, s := range build.Steps {
		if !slices.Contains(releaseconfig.Stages, s.Stage) || len(s.Exec) == 0 {
			return nil, fmt.Errorf("project %q: provider %s planned %q in stage %q", project.Name, build.Provider, s.Name, s.Stage)
		}
		b.add(s.Stage, Step{Name: s.Name, Exec: slices.Clone(s.Exec)})
	}
	for _, stage := range releaseconfig.Stages {
		if cmd, ok := project.Stages[stage]; ok {
			if extra := build.StageEnv[stage]; len(extra) > 0 {
				env := maps.Clone(extra)
				maps.Copy(env, cmd.Env)
				cmd.Env = env
			}
			b.add(stage, Step{Name: stage, Command: cmd})
		}
	}
	// The images are saved once the package stage's commands (bash) or
	// compose build have built them.
	images, err := b.saveImages(plan.OutputsDir)
	if err != nil {
		return nil, fmt.Errorf("project %q: %w", project.Name, err)
	}
	plan.Images = images
	b.coverage()

	for _, stage := range releaseconfig.Stages {
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
	for _, key := range releaseconfig.LifecycleHooks {
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
	project     releaseconfig.Project
	build       BuildSteps
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

// saveImages plans, at the end of the package stage, one docker save per
// image of the project's images, and returns the images saved. A {name, tags}
// entry is built as <name>:<version>: it is first given its other tags
// with docker tag, then saved with all of them, as 4.2.0 tagged it.
func (b *builder) saveImages(outputs string) ([]Image, error) {
	prefix := b.build.ImagePrefix
	var images []Image
	saved := func(ref string) bool {
		return slices.ContainsFunc(images, func(i Image) bool { return slices.Contains(i.Refs, ref) })
	}
	for _, bi := range b.project.Images {
		var source string
		var refs []string
		if bi.Ref != "" {
			ref := b.expand(bi.Ref)
			if !imageRefRe.MatchString(ref) {
				return nil, fmt.Errorf("images: %q expands to %q, not an image reference", bi.Ref, ref)
			}
			source, refs = ref, []string{ref}
		} else {
			tags, err := b.dockerTags(bi)
			if err != nil {
				return nil, fmt.Errorf("images %s: %w", bi.Name, err)
			}
			source = bi.Name + ":" + b.ctx.Version
			for _, t := range tags {
				refs = append(refs, bi.Name+":"+t)
			}
		}
		refs = slices.DeleteFunc(refs, saved)
		if len(refs) == 0 {
			continue
		}
		for _, ref := range refs {
			if !imageRefRe.MatchString(ref) {
				return nil, fmt.Errorf("images: %q is not an image reference", ref)
			}
			if ref != source {
				b.add("package", Step{Name: prefix + "tag " + ref, Exec: []string{"docker", "tag", source, ref}})
			}
		}
		img := Image{Ref: refs[0], Refs: refs, Tar: tarName(refs[0])}
		images = append(images, img)
		b.add("package", Step{Name: prefix + "save " + img.Ref, Exec: append([]string{"docker", "save", "-o", filepath.Join(outputs, img.Tar)}, refs...)})
	}
	return images, nil
}

// dockerTagRe is a valid docker image tag.
var dockerTagRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// dockerTags resolves the tag kinds of a {name, tags} entry for this
// release, as 4.2.0's docker preset did: version is always applied;
// channel only when the channel is not the default one; latest only on
// the default channel. Duplicates are dropped and every tag must be valid.
func (b *builder) dockerTags(img releaseconfig.BuildImage) ([]string, error) {
	kinds := img.Tags
	if len(kinds) == 0 {
		kinds = releaseconfig.DefaultDockerTags()
	}
	var tags []string
	for _, kind := range kinds {
		var t string
		switch kind {
		case "version":
			t = b.ctx.Version
		case "channel":
			if !b.ctx.defaultChannel() {
				t = b.ctx.Channel
			}
		case "latest":
			if b.ctx.defaultChannel() {
				t = "latest"
			}
		default:
			return nil, fmt.Errorf("unknown tag kind %q", kind)
		}
		if t == "" || slices.Contains(tags, t) {
			continue
		}
		if !dockerTagRe.MatchString(t) {
			return nil, fmt.Errorf("%s tag %q is not a valid docker tag ([A-Za-z0-9_][A-Za-z0-9_.-]*, at most 128 characters)", kind, t)
		}
		tags = append(tags, t)
	}
	return tags, nil
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

// coverage plans the floor check of the project's coverage at the end of
// the coverage stage.
func (b *builder) coverage() {
	c := b.project.Coverage
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
func describeFloors(f releaseconfig.CoverageThreshold) string {
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
