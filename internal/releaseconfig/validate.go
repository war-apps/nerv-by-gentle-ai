package releaseconfig

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig/yamlsection"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

// Stages are the pipeline stages, in run order.
var Stages = []string{"restore", "lint", "build", "test", "coverage", "package"}

// LifecycleHooks are the release steps a hook can attach to, besides
// before_<stage> and after_<stage>: prepare runs after the pipeline, before
// the artifact is written; fail runs when a stage or hook fails.
var LifecycleHooks = []string{"prepare", "fail"}

// Shells are the interpreters a Command can run under.
var Shells = []string{"node", "pwsh", "bash"}

// Bumps are the dependency bump levels.
var Bumps = []string{"patch", "minor", "major", "none"}

// BuildArgPlaceholders are the placeholders a compose-mode build.images
// reference may use: ${version}, ${channel} and ${tag}.
var BuildArgPlaceholders = []string{"version", "channel", "tag"}

// reservedEnvPrefix is the prefix of the variables NERV passes to hooks.
const reservedEnvPrefix = "NERV_RELEASE_"

var (
	nameRe        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	scopeRe       = regexp.MustCompile(`^[^\s():]+$`)
	envNameRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	placeholderRe = regexp.MustCompile(`\$\{([^}]*)\}`)
)

// finalizePaths runs after validation: it rewrites every path as a clean
// forward-slash path and derives the default changelog from the (now known
// to be safe) project path.
func (r *Release) finalizePaths() {
	for i := range r.Projects {
		p := &r.Projects[i]
		p.Path = normalize(p.Path)
		for j := range p.SharedPaths {
			p.SharedPaths[j] = normalize(p.SharedPaths[j])
		}
		if p.Changelog == "" {
			p.Changelog = r.defaultChangelog(p.Path)
		} else {
			p.Changelog = normalize(p.Changelog)
		}
		if b := p.Build; b != nil {
			b.File = normalize(b.File)
			for j := range b.VersionFiles {
				b.VersionFiles[j] = normalize(b.VersionFiles[j])
			}
			if b.Coverage != nil {
				b.Coverage.Dir = normalize(b.Coverage.Dir)
			}
		}
		for stage, c := range p.Stages {
			c.Workdir = normalize(c.Workdir)
			p.Stages[stage] = c
		}
		for _, cmds := range p.Hooks {
			for j := range cmds {
				cmds[j].Workdir = normalize(cmds[j].Workdir)
			}
		}
	}
}

// normalize turns a user path into a clean forward-slash path.
func normalize(p string) string { return yamlsection.NormalizePath(p) }

// validate returns one path-qualified message per problem.
func (r *Release) validate() []string {
	var v validator
	// The engine settings are the version engine's own, checked by it.
	var engine *versioning.ConfigError
	if err := r.engineSettings().WithDefaults().Validate(); errors.As(err, &engine) {
		for _, p := range engine.Problems {
			v.addf("%s.%s", SectionKey, p)
		}
	}

	byName := map[string]int{}
	for i, p := range r.Projects {
		at := fmt.Sprintf("release.projects[%d]", i)
		switch {
		case p.Name == "":
			v.addf("%s.name: required", at)
		case !nameRe.MatchString(p.Name):
			v.addf("%s.name: %q must match [a-z0-9][a-z0-9-]*", at, p.Name)
		default:
			if _, dup := byName[p.Name]; dup {
				v.addf("%s.name: duplicate project %q", at, p.Name)
			} else {
				byName[p.Name] = i
			}
		}
	}

	tagOwner := map[string]string{}
	refsOK := true
	for i, p := range r.Projects {
		at := fmt.Sprintf("release.projects[%d]", i)
		if p.Path == "" {
			v.addf("%s.path: required", at)
		} else {
			v.checkPath(at+".path", p.Path)
		}
		for j, sp := range p.SharedPaths {
			v.checkPath(fmt.Sprintf("%s.shared_paths[%d]", at, j), sp)
		}
		if p.Changelog != "" {
			v.checkPath(at+".changelog", p.Changelog)
		}
		for j, s := range p.Scopes {
			switch {
			case s == "":
				v.addf("%s.scopes[%d]: required", at, j)
			case !scopeRe.MatchString(s):
				v.addf("%s.scopes[%d]: %q is not a valid conventional-commit scope", at, j, s)
			}
		}
		v.checkTagFormat(at, p, tagOwner)

		seen := map[string]bool{}
		for j, d := range p.DependsOn {
			dat := fmt.Sprintf("%s.depends_on[%d]", at, j)
			switch _, known := byName[d.Project]; {
			case d.Project == "":
				v.addf("%s.project: required", dat)
				refsOK = false
			case d.Project == p.Name:
				v.addf("%s.project: a project cannot depend on itself", dat)
				refsOK = false
			case !known:
				v.addf("%s.project: unknown project %q", dat, d.Project)
				refsOK = false
			case seen[d.Project]:
				v.addf("%s.project: duplicate dependency on %q", dat, d.Project)
			}
			seen[d.Project] = true
			if !slices.Contains(Bumps, d.Bump) {
				v.addf("%s.bump: unknown bump %q (want %s)", dat, d.Bump, list(Bumps))
			}
		}
		v.checkBuild(at, p)
		v.checkCommands(at, p)
	}

	if refsOK && len(byName) == len(r.Projects) {
		if _, err := r.TopoOrder(); err != nil {
			v.addf("release.projects: %v", err)
		}
	}
	return v.problems
}

type validator struct{ problems []string }

func (v *validator) addf(format string, args ...any) {
	v.problems = append(v.problems, fmt.Sprintf(format, args...))
}

// checkPath rejects absolute paths and paths escaping the repository root,
// quoting the path as the user wrote it.
func (v *validator) checkPath(at, raw string) {
	if problem := yamlsection.PathProblem(at, raw); problem != "" {
		v.problems = append(v.problems, problem)
	}
}

func (v *validator) checkTagFormat(at string, p Project, owner map[string]string) {
	if !strings.Contains(p.TagFormat, "${version}") {
		v.addf("%s.tag_format: %q must contain ${version}", at, p.TagFormat)
	}
	for _, m := range placeholderRe.FindAllStringSubmatch(p.TagFormat, -1) {
		if m[1] != "version" {
			v.addf("%s.tag_format: unknown placeholder ${%s} (only ${version} is allowed)", at, m[1])
		}
	}
	if other, dup := owner[p.TagFormat]; dup {
		v.addf("%s.tag_format: %q is already used by project %q", at, p.TagFormat, other)
	} else {
		owner[p.TagFormat] = p.Name
	}
}

func (v *validator) checkCommands(at string, p Project) {
	for _, stage := range sortedKeys(p.Stages) {
		sat := at + ".stages." + stage
		if msg, removed := removedStages[stage]; removed {
			v.addf("%s: %s", sat, msg)
			continue
		}
		if !slices.Contains(Stages, stage) {
			v.addf("%s: unknown stage (want %s)", sat, list(Stages))
			continue
		}
		v.checkCommand(sat, p.Stages[stage])
	}
	for _, key := range sortedKeys(p.Hooks) {
		hat := at + ".hooks." + key
		if msg, removed := removedHooks[key]; removed {
			v.addf("%s: %s", hat, msg)
			continue
		}
		if !validHook(key) {
			v.addf("%s: unknown hook (want before_<stage>, after_<stage>, %s)", hat, list(LifecycleHooks))
			continue
		}
		for j, c := range p.Hooks[key] {
			v.checkCommand(fmt.Sprintf("%s[%d]", hat, j), c)
		}
	}
}

func (v *validator) checkCommand(at string, c Command) {
	if strings.TrimSpace(c.Run) == "" {
		v.addf("%s.run: required", at)
	}
	if !slices.Contains(Shells, c.Shell) {
		v.addf("%s.shell: unknown shell %q (want %s)", at, c.Shell, list(Shells))
	}
	if c.Workdir != "" {
		v.checkPath(at+".workdir", c.Workdir)
	}
	if c.Timeout != "" {
		switch d, err := time.ParseDuration(c.Timeout); {
		case err != nil:
			v.addf("%s.timeout: %q is not a duration such as 90s or 10m", at, c.Timeout)
		case d <= 0:
			v.addf("%s.timeout: %q must be positive", at, c.Timeout)
		}
	}
	for _, key := range sortedKeys(c.Env) {
		switch {
		case !envNameRe.MatchString(key):
			v.addf("%s.env.%s: not a valid environment variable name", at, key)
		case strings.HasPrefix(key, reservedEnvPrefix):
			v.addf("%s.env.%s: the %s prefix is reserved", at, key, reservedEnvPrefix)
		}
	}
}

func validHook(key string) bool {
	if slices.Contains(LifecycleHooks, key) {
		return true
	}
	for _, prefix := range []string{"before_", "after_"} {
		if stage, ok := strings.CutPrefix(key, prefix); ok && slices.Contains(Stages, stage) {
			return true
		}
	}
	return false
}

// list renders "a, b or c".
func list(items []string) string {
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

// placeholders renders names as ${name}.
func placeholders(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "${" + n + "}"
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// checkPercent reports a coverage floor that is not a percentage (NaN fails
// every comparison and ±Inf is no percentage either).
func (v *validator) checkPercent(at string, c float64) {
	if math.IsNaN(c) || c < 0 || c > 100 {
		v.addf("%s: %v must be between 0 and 100", at, c)
	}
}
