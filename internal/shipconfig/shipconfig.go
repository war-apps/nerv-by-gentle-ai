// Package shipconfig reads the release: section of a repository's
// project-scope .nerv/nerv.yaml: the typed, validated configuration behind
// `nerv version` and `nerv ship`.
//
// The rest of nerv.yaml is owned by internal/config, a line-based editor
// that cannot hold lists of maps. This package parses the file with
// gopkg.in/yaml.v3 but only looks at the release: key and never writes the
// file. The section is project scope only: there is no user-scope merge.
//
// Every path in the section is relative to the repository root and written
// with forward slashes; Load normalises backslashes and rejects absolute
// paths and paths that escape the repository.
package shipconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultSemanticReleaseVersion is the semantic-release version installed in
// the NERV cache when release.semantic_release.version is not set.
const DefaultSemanticReleaseVersion = "24.2.3"

// DefaultNuGetSource is the push target when release.projects[].nuget.source
// is not set.
const DefaultNuGetSource = "https://api.nuget.org/v3/index.json"

// ErrNotConfigured is returned by Load when the repository has no
// .nerv/nerv.yaml or the file has no release: key.
var ErrNotConfigured = errors.New("release: not configured in .nerv/nerv.yaml")

// Release is the release: section.
type Release struct {
	// SemanticRelease pins the semantic-release engine.
	SemanticRelease SemanticRelease `yaml:"semantic_release"`
	// Projects are the releasable units. A single-repo setup is one project
	// with path ".". Required, at least one.
	Projects []Project `yaml:"projects"`
}

// SemanticRelease pins the semantic-release version and the extra npm
// packages installed next to it in the NERV cache (never in the repo).
type SemanticRelease struct {
	// Version is an exact semantic-release version.
	// Default: DefaultSemanticReleaseVersion.
	Version string `yaml:"version"`
	// ExtraPlugins are pinned npm specs ("name@version") the repository's
	// own semantic-release configuration needs beyond the plugins bundled
	// with semantic-release, e.g. "@semantic-release/exec@6.0.3".
	ExtraPlugins []string `yaml:"extra_plugins"`
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
	// TagFormat is the semantic-release tagFormat; it must contain
	// ${version} and no other placeholder, and be unique across projects.
	// Default: "v${version}" with one project, "<name>-v${version}" otherwise.
	TagFormat string `yaml:"tag_format"`
	// Changelog is the CHANGELOG file. Default: <path>/CHANGELOG.md.
	Changelog string `yaml:"changelog"`
	// DependsOn lists the projects whose new release bumps this one.
	DependsOn []Dependency `yaml:"depends_on"`
	// Presets enables built-in stages and publishers: a subset of
	// dotnet, node, docker, nuget, npm.
	Presets []string `yaml:"presets"`
	// Dotnet configures the dotnet preset.
	Dotnet *Dotnet `yaml:"dotnet"`
	// Node configures the node preset.
	Node *Node `yaml:"node"`
	// Docker configures the docker preset.
	Docker *Docker `yaml:"docker"`
	// NuGet configures the nuget preset.
	NuGet *NuGet `yaml:"nuget"`
	// Npm configures the npm preset.
	Npm *Npm `yaml:"npm"`
	// Stages overrides or adds the command of a pipeline stage, keyed by one
	// of Stages (restore, lint, build, test, coverage, package, publish).
	Stages map[string]Command `yaml:"stages"`
	// Hooks run scripts around stages and at semantic-release lifecycle
	// steps, keyed by before_<stage>, after_<stage>, prepare, publish,
	// success or fail. Each key runs its commands in order.
	Hooks map[string][]Command `yaml:"hooks"`
}

// Dependency makes a project follow another project's releases.
type Dependency struct {
	// Project names another project of the section. Required.
	Project string `yaml:"project"`
	// Bump is applied to the dependent when the dependency releases:
	// patch, minor or none. Default: patch.
	Bump string `yaml:"bump"`
}

// Dotnet configures the dotnet preset (restore, format check, build, test,
// coverage, pack, and the <Version>/<PackageReleaseNotes> csproj write).
type Dotnet struct {
	// ProjectFile is the .csproj that receives the version. Required by the
	// dotnet preset.
	ProjectFile string `yaml:"project_file"`
	// Solution is the .sln/.slnx the stages run against. Default: empty,
	// meaning ProjectFile.
	Solution string `yaml:"solution"`
	// CoverageThreshold is the minimum line coverage percentage (0-100);
	// 0 disables the check.
	CoverageThreshold float64 `yaml:"coverage_threshold"`
	// Configuration is the build configuration. Default: Release.
	Configuration string `yaml:"configuration"`
}

// Node configures the node preset.
type Node struct {
	// PackageManager is npm, pnpm or yarn. Default: npm.
	PackageManager string `yaml:"package_manager"`
}

// Docker configures the docker preset.
type Docker struct {
	// Images are built, tagged and pushed in order. Required (at least one)
	// by the docker preset.
	Images []DockerImage `yaml:"images"`
}

// DockerImage is one image the docker preset builds.
type DockerImage struct {
	// Name is the image repository, e.g. ghcr.io/org/api. Required.
	Name string `yaml:"name"`
	// Dockerfile is the Dockerfile path. Default: Dockerfile.
	Dockerfile string `yaml:"dockerfile"`
	// Context is the build context directory. Default: ".".
	Context string `yaml:"context"`
	// BuildArgs are passed as --build-arg; values may use the ${version},
	// ${channel} and ${tag} placeholders.
	BuildArgs map[string]string `yaml:"build_args"`
	// Tags selects the image tags: version (the release version), channel
	// (the release channel, e.g. alpha or rc) and latest (only applied on a
	// release of the default channel). Default: [version, channel, latest].
	Tags []string `yaml:"tags"`
	// Push pushes the tags after the build. Default: true; Load always sets
	// it, read it through ShouldPush.
	Push *bool `yaml:"push"`
}

// ShouldPush reports whether the image's tags are pushed (true when unset).
func (i DockerImage) ShouldPush() bool { return i.Push == nil || *i.Push }

// NuGet configures the nuget preset (dotnet nuget push).
type NuGet struct {
	// Source is the push source. Default: DefaultNuGetSource.
	Source string `yaml:"source"`
	// APIKeyEnv is the environment variable holding the API key.
	// Default: NUGET_API_KEY.
	APIKeyEnv string `yaml:"api_key_env"`
}

// Npm configures the npm preset (npm publish).
type Npm struct {
	// Registry is the publish registry. Default: empty, meaning npm's own
	// configuration.
	Registry string `yaml:"registry"`
	// Access is public or restricted. Default: public.
	Access string `yaml:"access"`
	// DistTag is the dist-tag. Default: empty, meaning the release channel
	// (latest on the default channel).
	DistTag string `yaml:"dist_tag"`
}

// Command is a script run by a stage override or a hook.
type Command struct {
	// Run is the command line or script. Required.
	Run string `yaml:"run"`
	// Shell runs Run: node, pwsh or bash. Default: bash.
	Shell string `yaml:"shell"`
	// Env adds environment variables. The NERV_RELEASE_ prefix is reserved
	// for the variables NERV itself passes (version, channel, tag, ...).
	Env map[string]string `yaml:"env"`
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

// Load reads the release: section of <repoRoot>/.nerv/nerv.yaml, applies
// defaults and validates it. It returns ErrNotConfigured when the file or
// the key is absent, and an error listing every problem, each prefixed by
// its path in the section (e.g. release.projects[1].depends_on[0].project),
// when the section is invalid.
func Load(repoRoot string) (*Release, error) {
	path := filepath.Join(repoRoot, ".nerv", "nerv.yaml")
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

// parse decodes the release: key of a nerv.yaml text.
func parse(data []byte) (*Release, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("release: %w", err)
	}
	node := releaseNode(&doc)
	if node == nil {
		return nil, ErrNotConfigured
	}
	var rel Release
	switch {
	case node.Tag == "!!null":
		// "release:" with no value: present but empty; validation says
		// what is missing.
	case node.Kind != yaml.MappingNode:
		return nil, &ValidationError{Problems: []string{"release: must be a mapping"}}
	default:
		if problems := unknownFields(node, typeOfRelease, "release"); len(problems) > 0 {
			return nil, &ValidationError{Problems: problems}
		}
		if err := node.Decode(&rel); err != nil {
			msg := strings.TrimPrefix(err.Error(), "yaml: unmarshal errors:\n")
			var lines []string
			for _, l := range strings.Split(msg, "\n") {
				lines = append(lines, "release: "+strings.TrimSpace(l))
			}
			return nil, &ValidationError{Problems: lines}
		}
	}
	rel.applyDefaults()
	if problems := rel.validate(); len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	rel.finalizePaths()
	return &rel, nil
}

// releaseNode returns the value of the top-level release: key, or nil.
func releaseNode(doc *yaml.Node) *yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "release" {
			return root.Content[i+1]
		}
	}
	return nil
}

// ValidationError lists every problem found in the release: section, one
// path-qualified message per entry.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid release configuration:\n  " + strings.Join(e.Problems, "\n  ")
}
