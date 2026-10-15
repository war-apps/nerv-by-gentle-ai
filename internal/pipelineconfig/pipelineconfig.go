// Package pipelineconfig reads the pipeline: section of a repository's
// project-scope .nerv/nerv.yaml: the typed, validated configuration behind
// `nerv pipeline`, a standalone copy of dim.comprobantes.api's GitHub Actions
// CI/CD (validate, test, release, docker, sync) plus a Docker deploy.
//
// Like the release: section (internal/shipconfig), the pipeline: block is cut
// out of the file and parsed alone with gopkg.in/yaml.v3, every key is
// checked strictly, and every problem is reported at once with its path. The
// file read is always the local working-tree .nerv/nerv.yaml. Paths are
// relative to the repository root and written with forward slashes; Load
// normalises backslashes and rejects absolute and escaping paths.
//
// The section holds names, never secret values: credentials are named by
// the environment variable that holds them (registry.*_env), and deploy
// targets come from environment variables or Infisical (deploy.source).
package pipelineconfig

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig/yamlsection"
	"gopkg.in/yaml.v3"
)

// ErrNotConfigured is returned by Load when the repository has no
// .nerv/nerv.yaml or the file has no pipeline: key.
var ErrNotConfigured = errors.New("pipeline: not configured in .nerv/nerv.yaml")

// Pipeline is the pipeline: section.
type Pipeline struct {
	// Dotnet configures the validate and test steps. Required.
	Dotnet Dotnet `yaml:"dotnet"`
	// Branches are the semantic-release branches, in semantic-release order.
	// Default: DefaultBranches().
	Branches []Branch `yaml:"branches"`
	// Registry is where the images are pushed.
	Registry Registry `yaml:"registry"`
	// Images are built and pushed by the docker step, in order. Required, at
	// least one.
	Images []Image `yaml:"images"`
	// Sync configures the merge-back of released branches.
	Sync Sync `yaml:"sync"`
	// Deploy configures the deploy step; nil (absent) means no deploy.
	Deploy *Deploy `yaml:"deploy"`
	// Release configures the release step.
	Release Release `yaml:"release"`
}

// Release configures the release step (dim's release job).
type Release struct {
	// VersionFiles are MSBuild project files (repository-relative) that get
	// <Version> and <PackageReleaseNotes> (the release link) written before
	// semantic-release, so the release commit holds them (dim's
	// scripts/bump-csproj.sh on src/Api/Dim.Comprobantes.Api.csproj).
	VersionFiles []string `yaml:"version_files"`
}

// Dotnet configures the dotnet commands of the validate and test steps.
type Dotnet struct {
	// Solution is the .sln/.slnx every dotnet command runs against. Required.
	Solution string `yaml:"solution"`
	// Configuration is the build configuration.
	// Default: DefaultDotnetConfiguration.
	Configuration string `yaml:"configuration"`
	// Runsettings is passed to dotnet test --settings (coverage collector and
	// thresholds). Default: empty, meaning no --settings.
	Runsettings string `yaml:"runsettings"`
	// WarningsAsErrors builds with -warnaserror. Default: true; read it
	// through TreatWarningsAsErrors.
	WarningsAsErrors *bool `yaml:"warnings_as_errors"`
	// Env adds environment variables to every dotnet command (literal
	// values, never secrets).
	Env map[string]string `yaml:"env"`
}

// TreatWarningsAsErrors reports whether the build uses -warnaserror (true
// when unset).
func (d Dotnet) TreatWarningsAsErrors() bool { return d.WarningsAsErrors == nil || *d.WarningsAsErrors }

// Branch is one semantic-release branch.
type Branch struct {
	// Name is the branch name or a glob such as release/*. Required, unique.
	Name string `yaml:"name"`
	// Prerelease is the prerelease id (alpha, rc, ...); empty means a release
	// branch.
	Prerelease string `yaml:"prerelease"`
	// Channel is the distribution channel, also the moving image tag.
	// Default: Prerelease (empty on a release branch: the default channel).
	Channel string `yaml:"channel"`
}

// IsPrerelease reports whether the branch publishes prereleases.
func (b Branch) IsPrerelease() bool { return b.Prerelease != "" }

// Registry names the image registry and the environment variables that hold
// its credentials.
type Registry struct {
	// Host is the registry host; every image name starts with it.
	// Default: DefaultRegistryHost.
	Host string `yaml:"host"`
	// UsernameEnv names the variable holding the login user.
	// Default: DefaultRegistryUsernameEnv.
	UsernameEnv string `yaml:"username_env"`
	// TokenEnv names the variable holding the login token.
	// Default: DefaultRegistryTokenEnv.
	TokenEnv string `yaml:"token_env"`
}

// Image is one image the docker step builds and pushes.
type Image struct {
	// Name is the image repository, <registry host>/<path>, without a tag.
	// Required, unique; lowercased on load as dim's workflow does.
	Name string `yaml:"name"`
	// Title is the org.opencontainers.image.title label.
	// Default: the last path component of Name.
	Title string `yaml:"title"`
	// Dockerfile is the Dockerfile path. Default: DefaultDockerfile.
	Dockerfile string `yaml:"dockerfile"`
	// Context is the build context directory. Default: DefaultDockerContext.
	Context string `yaml:"context"`
	// Platform is the --platform list (comma separated).
	// Default: DefaultPlatform.
	Platform string `yaml:"platform"`
	// BuildArgs are passed as --build-arg; values may use ${version},
	// ${channel} and ${tag}.
	BuildArgs map[string]string `yaml:"build_args"`
	// ComputedArgs are build arguments whose value is the trimmed standard
	// output of a command run at build time. When the image has any, a
	// failed build is retried once with the arguments computed again.
	ComputedArgs map[string]ComputedArg `yaml:"computed_args"`
	// Tags selects the image tags: version, channel (not on the default
	// channel) and latest (main only). Default: all three.
	Tags []string `yaml:"tags"`
	// Required makes a failure of this image fail the step. Default: true;
	// read it through IsRequired.
	Required *bool `yaml:"required"`
}

// IsRequired reports whether a failure of the image fails the step (true
// when unset).
func (i Image) IsRequired() bool { return i.Required == nil || *i.Required }

// ComputedArg is a build argument computed by a command.
type ComputedArg struct {
	// Run is the command line. Required.
	Run string `yaml:"run"`
	// Shell runs Run: node, pwsh or bash. Default: DefaultShell.
	Shell string `yaml:"shell"`
}

// Sync configures the merge-back after a release: the main branch (the
// first release branch) and every branch matching Release are merged
// --no-ff into Develop, and main is merged into every branch matching
// Release.
type Sync struct {
	// Enabled turns the sync step on. Default: true; read it through
	// IsEnabled.
	Enabled *bool `yaml:"enabled"`
	// Develop is the branch that receives the merges.
	// Default: DefaultSyncDevelop.
	Develop string `yaml:"develop"`
	// Release is the pattern of the release-candidate branches.
	// Default: DefaultSyncRelease.
	Release string `yaml:"release"`
}

// IsEnabled reports whether the sync step runs (true when unset).
func (s Sync) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

// Deploy configures the deploy step.
type Deploy struct {
	// Source is where the deploy data comes from: env or infisical.
	// Default: DefaultDeploySource.
	Source string `yaml:"source"`
	// Infisical configures source infisical.
	Infisical *Infisical `yaml:"infisical"`
	// Environments are deployed in this order.
	// Default: DefaultEnvironments().
	Environments []string `yaml:"environments"`
	// SSH configures the connection to the Docker hosts.
	SSH SSH `yaml:"ssh"`
	// Verify configures the check after each deploy.
	Verify Verify `yaml:"verify"`
}

// Infisical locates the deploy data in Infisical (read through the user's
// `infisical login` CLI session).
type Infisical struct {
	// ProjectID is the Infisical project. Required by source infisical.
	ProjectID string `yaml:"project_id"`
	// AppPath is the application's folder, e.g. /my-app. Required by source
	// infisical.
	AppPath string `yaml:"app_path"`
	// Domain is the Infisical URL. Default: empty, meaning the CLI
	// session's own domain.
	Domain string `yaml:"domain"`
}

// SSH configures the connection to the Docker hosts.
type SSH struct {
	// Auth is password (DOCKER_SSH_PASS) or key (KeyFile).
	// Default: DefaultSSHAuth.
	Auth string `yaml:"auth"`
	// KeyFile is the private key file. Required by auth key.
	KeyFile string `yaml:"key_file"`
	// KnownHosts is the known_hosts file; host keys are always checked
	// strictly. Default: DefaultSSHKnownHosts.
	KnownHosts string `yaml:"known_hosts"`
	// Port is the SSH port. Default: DefaultSSHPort.
	Port int `yaml:"port"`
	// ConnectTimeout bounds the connection as a Go duration.
	// Default: DefaultSSHConnectTimeout; read it through
	// ConnectTimeoutDuration.
	ConnectTimeout string `yaml:"connect_timeout"`
}

// ConnectTimeoutDuration returns the validated connect timeout.
func (s SSH) ConnectTimeoutDuration() time.Duration { return duration(s.ConnectTimeout) }

// Verify configures the check after each deploy.
type Verify struct {
	// Healthy waits for the container's (healthy) status. Default: true;
	// read it through CheckHealthy.
	Healthy *bool `yaml:"healthy"`
	// VersionLabel is the image label compared with the deployed version.
	// Default: DefaultVersionLabel.
	VersionLabel string `yaml:"version_label"`
	// Timeout bounds the wait as a Go duration. Default:
	// DefaultVerifyTimeout; read it through TimeoutDuration.
	Timeout string `yaml:"timeout"`
}

// CheckHealthy reports whether the deploy waits for (healthy) (true when
// unset).
func (v Verify) CheckHealthy() bool { return v.Healthy == nil || *v.Healthy }

// TimeoutDuration returns the validated verify timeout.
func (v Verify) TimeoutDuration() time.Duration { return duration(v.Timeout) }

func duration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// Branch returns the first branch entry whose name or glob matches the git
// branch name (a * does not cross a /).
func (p *Pipeline) Branch(name string) (Branch, bool) {
	for _, b := range p.Branches {
		if ok, err := path.Match(b.Name, name); err == nil && ok {
			return b, true
		}
	}
	return Branch{}, false
}

// MainBranch is the first release branch (without prerelease): the branch
// whose images get the latest tag and that the sync step merges back.
func (p *Pipeline) MainBranch() string {
	for _, b := range p.Branches {
		if !b.IsPrerelease() {
			return b.Name
		}
	}
	return ""
}

// Load reads the pipeline: section of <repoRoot>/.nerv/nerv.yaml, applies
// defaults and validates it. It returns ErrNotConfigured when the file or
// the key is absent, and a *ValidationError listing every problem, each
// prefixed by its path in the section (e.g. pipeline.images[1].name).
func Load(repoRoot string) (*Pipeline, error) {
	file := filepath.Join(repoRoot, ".nerv", "nerv.yaml")
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotConfigured
	}
	if err != nil {
		return nil, fmt.Errorf("pipeline: %w", err)
	}
	p, err := parse(data)
	if err != nil {
		if errors.Is(err, ErrNotConfigured) {
			return nil, err
		}
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return p, nil
}

var typeOfPipeline = reflect.TypeFor[Pipeline]()

// parse decodes the pipeline: key of a nerv.yaml text.
func parse(data []byte) (*Pipeline, error) {
	node, err := yamlsection.Locate(data, "pipeline")
	var problem *yamlsection.ProblemError
	switch {
	case errors.Is(err, yamlsection.ErrAbsent):
		return nil, ErrNotConfigured
	case errors.As(err, &problem):
		return nil, &ValidationError{Problems: []string{problem.Problem}}
	case err != nil:
		return nil, err
	}
	var p Pipeline
	var problems []string
	switch {
	case node.ShortTag() == "!!null":
		// "pipeline:" with no value: present but empty; validation says
		// what is missing.
	case node.Kind != yaml.MappingNode:
		return nil, &ValidationError{Problems: []string{"pipeline: must be a mapping"}}
	default:
		unknown, err := yamlsection.UnknownFields(node, typeOfPipeline, "pipeline")
		if err != nil {
			return nil, &ValidationError{Problems: []string{err.Error()}}
		}
		problems = unknown
		if err := node.Decode(&p); err != nil {
			// A partially decoded section would produce misleading
			// semantic errors; stop at the decoding problems.
			return nil, &ValidationError{Problems: append(problems, yamlsection.DecodeProblems("pipeline", err)...)}
		}
	}
	p.applyDefaults()
	problems = append(problems, p.validate()...)
	if len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	p.finalizePaths()
	return &p, nil
}

// ValidationError lists every problem found in the pipeline: section, one
// path-qualified message per entry.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "invalid pipeline configuration:\n  " + strings.Join(e.Problems, "\n  ")
}
