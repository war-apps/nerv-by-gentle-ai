// Package releaseconfig reads the release: section of a repository's
// project-scope .ai-workflow.yml: the typed, validated configuration behind
// `nerv version next` and `nerv release`. The section selects the providers
// of the release extension's slots (version: the version engine; build: a
// project's build), holds their keys under providers: (each provider
// checks its own when it is called, internal/releaseplan) and the one
// project list the engine and the build use.
//
// The rest of .ai-workflow.yml is owned by internal/config, a line-based editor
// that cannot hold lists of maps. This package cuts the top-level release:
// block out of the file and parses only that block with gopkg.in/yaml.v3,
// so a YAML error elsewhere in the file does not matter (and anchors defined
// outside the block cannot be used inside it). It never writes the file.
// The section is project scope only: there is no user-scope merge.
//
// Every path in the section is relative to the repository root and written
// with forward slashes; Load normalises backslashes and rejects absolute
// paths and paths that escape the repository.
package releaseconfig

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig/yamlsection"
	"gopkg.in/yaml.v3"
)

// SectionKey is the top-level key of the section in .ai-workflow.yml.
const SectionKey = "release"

// ErrNotConfigured is returned by Load when the repository has no
// .ai-workflow.yml or the file has no release: key.
var ErrNotConfigured = errors.New("release: not configured in .ai-workflow.yml")

// Release is the release: section: the providers of the release
// extension's two slots, their config, and the projects.
type Release struct {
	// Version is the version engine, the provider of the version slot.
	// Default: the slot's default, conventional-commits.
	Version string `yaml:"version"`
	// Build is the build provider of every project that names none
	// (Project.Build), such as docker-compose or bash. nerv release
	// requires one per project (CheckBuild); other commands ignore it.
	Build string `yaml:"build"`
	// Providers are the providers' keys, by provider name
	// (release.providers.<provider>.<key>), as each provider's
	// provider.yaml declares them: the version engine's (branches,
	// release_rules, tag_format, changelog, ... of conventional-commits)
	// and the build providers' (a project overrides those under its own
	// providers). nerv checks them against the provider when it calls it.
	Providers map[string]map[string]any `yaml:"providers"`
	// Projects are the releasable units. A single-repo setup is one project
	// with path ".". nerv release needs at least one (CheckBuild); without any,
	// nerv version next versions the whole repository.
	Projects []Project `yaml:"projects"`
}

// Project is one independently versioned unit of the repository.
type Project struct {
	// Name identifies the project: unique, [a-z0-9][a-z0-9-]*. Required.
	Name string `yaml:"name"`
	// Path is the project's directory. A commit touching it counts for the
	// project. Required; "." is the whole repository.
	Path string `yaml:"path"`
	// SharedPaths are extra paths or globs whose changes also count for the
	// project (shared libraries, build files).
	SharedPaths []string `yaml:"shared_paths"`
	// Scopes are the conventional-commit scopes whose commits count for the
	// project wherever they touch. Default: [Name].
	Scopes []string `yaml:"scopes"`
	// TagFormat is the project's tag; it must contain ${version} and no
	// other placeholder, and be unique across projects. Default: the
	// version engine's (conventional-commits: its tag_format with one
	// project, "<name>-" + it otherwise).
	TagFormat string `yaml:"tag_format"`
	// Changelog is the CHANGELOG file. Default: the version engine's
	// (conventional-commits: <path>/<changelog.file>).
	Changelog string `yaml:"changelog"`
	// DependsOn lists the projects whose new release bumps this one.
	DependsOn []Dependency `yaml:"depends_on"`
	// Build is the project's build provider (docker-compose, bash),
	// overriding the section's build. nerv release requires one
	// (CheckBuild); other commands ignore it. Read it through
	// BuildProvider.
	Build string `yaml:"build"`
	// Providers override the section's providers' keys for this project
	// (release.projects[].providers.<provider>.<key>).
	Providers map[string]map[string]any `yaml:"providers"`
	// Images are the image references docker save packages into the
	// artifact after the package stage, recorded in its manifest so
	// deploy can push them; they may use ${version}, ${channel} and
	// ${tag}. docker-compose builds them with docker compose build; with
	// bash the project's own commands (the package stage, usually) must
	// build them.
	Images []BuildImage `yaml:"images"`
	// VersionFiles are MSBuild project files that receive the version in
	// the release clone before the pipeline.
	VersionFiles []string `yaml:"version_files"`
	// Coverage checks coverage floors over Cobertura reports after the
	// coverage stage.
	Coverage *Coverage `yaml:"coverage"`
	// Stages are the commands of the pipeline stages, keyed by one of
	// Stages (restore, lint, build, test, coverage, package), as the build
	// provider takes them: bash runs every stage's; docker-compose only
	// restore, lint and coverage (it runs build, test and package itself).
	Stages map[string]Command `yaml:"stages"`
	// Submodules runs git submodule update --init --recursive in the
	// release clone before the restore stage (the clone has no submodules
	// otherwise). Default: false.
	Submodules bool `yaml:"submodules"`
	// Hooks run scripts around stages and at the release lifecycle steps,
	// keyed by before_<stage>, after_<stage>, prepare (after the pipeline,
	// before the artifact is written) or fail. Each key runs its commands
	// in order.
	Hooks map[string][]Command `yaml:"hooks"`
}

// Dependency makes a project follow another project's releases.
type Dependency struct {
	// Project names another project of the section. Required.
	Project string `yaml:"project"`
	// Bump is applied to the dependent when the dependency releases:
	// patch, minor, major or none. Default: DefaultBump.
	Bump string `yaml:"bump"`
}

// CoverageThreshold are coverage floors, as coverlet's ThresholdType names
// them: line, branch and method percentages (0-100), over every report
// together. 0 leaves a metric unchecked.
type CoverageThreshold struct {
	Line   float64
	Branch float64
	Method float64
}

// Enabled reports whether any floor is checked.
func (c CoverageThreshold) Enabled() bool { return c.Line > 0 || c.Branch > 0 || c.Method > 0 }

// Command is a script run by a stage or a hook.
type Command struct {
	// Run is the command line or script. Required.
	Run string `yaml:"run"`
	// Shell runs Run: node, pwsh or bash. Default: DefaultShell.
	Shell string `yaml:"shell"`
	// Env adds environment variables. The NERV_RELEASE_ prefix is reserved
	// for the variables NERV itself passes (version, channel, tag, ...).
	Env map[string]string `yaml:"env"`
	// Workdir is the working directory, relative to the repository root.
	// Default: empty, meaning the repository root.
	Workdir string `yaml:"workdir"`
	// Timeout bounds the command as a Go duration (90s, 10m, 1h30m); it
	// must be positive. Default: empty, meaning no timeout. Read it through
	// TimeoutDuration.
	Timeout string `yaml:"timeout"`
}

// TimeoutDuration returns the command's timeout, 0 when it has none. Load
// has validated it.
func (c Command) TimeoutDuration() time.Duration {
	d, err := time.ParseDuration(c.Timeout)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// BuildProvider is the project's build provider: its own build, else the
// section's; empty when neither names one.
func (r *Release) BuildProvider(p Project) string {
	if p.Build != "" {
		return p.Build
	}
	return r.Build
}

// Project returns the project called name.
func (r *Release) Project(name string) (Project, bool) {
	for _, p := range r.Projects {
		if p.Name == name {
			return p, true
		}
	}
	return Project{}, false
}

// Load reads the release: section of <repoRoot>/.ai-workflow.yml, applies
// defaults and validates it. It returns ErrNotConfigured when the file or
// the key is absent, and an error listing every problem, each prefixed by
// its path in the section (e.g. release.projects[1].depends_on[0].project),
// when the section is invalid. A file that still has the removed release:
// or pipeline: section is refused (RemovedSections), with what to do, and
// so is a key that moved (an engine key under release:, a build: mapping),
// naming its new place.
func Load(repoRoot string) (*Release, error) {
	path := paths.ProjectConfig(repoRoot)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotConfigured
	}
	if err != nil {
		return nil, fmt.Errorf("release: %w", err)
	}
	rel, err := parse(data)
	if err != nil {
		if errors.Is(err, ErrNotConfigured) {
			return nil, err
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rel, nil
}

// parse decodes the release: key of a .ai-workflow.yml text. Unknown keys and
// semantic problems are reported together; only a section that cannot be
// decoded at all (or that expands past the alias budget) stops early.
func parse(data []byte) (*Release, error) {
	if removed := RemovedSections(data); len(removed) > 0 {
		return nil, &ValidationError{Problems: removed}
	}
	node, err := locateRelease(data)
	if err != nil {
		return nil, err
	}
	var rel Release
	var problems []string
	switch {
	case node.ShortTag() == "!!null":
		// "release:" with no value: present but empty; validation says
		// what is missing.
	case node.Kind != yaml.MappingNode:
		return nil, &ValidationError{Problems: []string{"release: must be a mapping"}}
	default:
		problems = takeRemovedKeys(node)
		unknown, err := unknownFields(node)
		if err != nil {
			return nil, &ValidationError{Problems: []string{err.Error()}}
		}
		problems = append(problems, unknown...)
		if err := node.Decode(&rel); err != nil {
			problems = append(problems, yamlsection.DecodeProblems(SectionKey, err)...)
			// A partially decoded section would produce misleading
			// semantic errors; stop at the decoding problems.
			return nil, &ValidationError{Problems: problems}
		}
	}
	rel.applyDefaults()
	problems = append(problems, rel.validate()...)
	if len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	rel.finalizePaths()
	return &rel, nil
}

// ValidationError lists every problem found in the release: section, one
// path-qualified message per entry.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid release configuration:\n  " + strings.Join(e.Problems, "\n  ")
}
