package pipelineconfig

import (
	"path"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig/yamlsection"
)

// The documented defaults of the pipeline: section (docs/pipeline.md). Each
// one is taken from dim.comprobantes.api's CI/CD (.github/workflows/ci-cd.yml)
// or its deploy helper (update-deploy.sh) unless noted otherwise.
const (
	// DefaultDotnetConfiguration is the build configuration
	// (ci-cd.yml: dotnet build --configuration Release).
	DefaultDotnetConfiguration = "Release"
	// DefaultRegistryHost is the image registry (ci-cd.yml: REGISTRY).
	DefaultRegistryHost = "ghcr.io"
	// DefaultRegistryUsernameEnv names the variable with the registry user
	// (dim's CI uses github.actor; a local run needs a variable).
	DefaultRegistryUsernameEnv = "GHCR_USER"
	// DefaultRegistryTokenEnv names the variable with the registry token
	// (dim's CI uses GITHUB_TOKEN; a local run needs a variable).
	DefaultRegistryTokenEnv = "GHCR_TOKEN"
	// DefaultDockerfile is the Dockerfile of an image.
	DefaultDockerfile = "Dockerfile"
	// DefaultDockerContext is the build context (ci-cd.yml: context: .).
	DefaultDockerContext = "."
	// DefaultPlatform is the build platform (ci-cd.yml: platforms).
	DefaultPlatform = "linux/amd64"
	// DefaultShell runs computed build arguments.
	DefaultShell = "bash"
	// DefaultSyncDevelop receives the merge-back (ci-cd.yml: sync-to-develop).
	DefaultSyncDevelop = "develop"
	// DefaultSyncRelease is the pattern of the branches main is merged into
	// (ci-cd.yml: sync-main-to-release).
	DefaultSyncRelease = "release/*"
	// DefaultDeploySource reads the deploy data from environment variables.
	DefaultDeploySource = "env"
	// DefaultSSHAuth is password authentication (update-deploy.sh: sshpass).
	DefaultSSHAuth = "password"
	// DefaultSSHPort is the SSH port (update-deploy.sh: reachable on 22).
	DefaultSSHPort = 22
	// DefaultSSHKnownHosts is the known_hosts file. dim pins its hosts in
	// ~/.config/update-deploy/known_hosts; the generic default is the user's
	// own file.
	DefaultSSHKnownHosts = "~/.ssh/known_hosts"
	// DefaultSSHConnectTimeout bounds the connection
	// (update-deploy.sh: ConnectTimeout=10).
	DefaultSSHConnectTimeout = "10s"
	// DefaultVersionLabel is the label holding the image version
	// (ci-cd.yml labels; update-deploy.sh verify).
	DefaultVersionLabel = "org.opencontainers.image.version"
	// DefaultVerifyTimeout bounds the wait for (healthy): dim's images
	// declare HEALTHCHECK --start-period=20s --interval=30s --retries=3,
	// at most 110s to become unhealthy.
	DefaultVerifyTimeout = "120s"
	// DeployEnvPrefix starts every deploy variable of source env.
	DeployEnvPrefix = "NERV_DEPLOY"
)

// DefaultBranches returns the branches used when branches is not set:
// main; develop (alpha); release/* (rc on the beta channel, as dim's
// release/rc); hotfix/* (beta, a fixed id instead of dim's
// "prerelease: true", which breaks on branch names with a /). Each call
// returns a fresh slice.
func DefaultBranches() []Branch {
	return []Branch{
		{Name: "main"},
		{Name: "develop", Prerelease: "alpha", Channel: "alpha"},
		{Name: "release/*", Prerelease: "rc", Channel: "beta"},
		{Name: "hotfix/*", Prerelease: "beta", Channel: "beta"},
	}
}

// DefaultEnvironments returns the deploy order used when environments is
// not set: dim's processing order (update-deploy.sh table). Each call
// returns a fresh slice.
func DefaultEnvironments() []string {
	return []string{"production-green", "production-blue", "staging", "development"}
}

// DeployHostKeys are the deploy keys of an environment's Docker host (the
// environment root in Infisical).
func DeployHostKeys() []string {
	return []string{"DOCKER_SSH_HOST", "DOCKER_SSH_USER", "DOCKER_SSH_PASS", "DOCKER_COMPOSE_PATH", "DOCKER_COMPOSE_SCRIPT"}
}

// DeployAppKeys are the deploy keys of the application in an environment
// (the app folder in Infisical).
func DeployAppKeys() []string {
	return []string{
		"DOCKER_CONTAINER_NAME", "DOCKER_CONTAINER_VARIANT", "DOCKER_DEPLOY_ENABLED",
		"DOCKER_DEPLOY_STRATEGY", "DOCKER_IMAGE", "DOCKER_IMAGE_VERSION",
	}
}

// DeployEnvVar is the variable holding a deploy key for an environment with
// source env: NERV_DEPLOY_<ENVIRONMENT>_<KEY>, the environment upper-cased
// with - turned into _ (production-blue, DOCKER_SSH_HOST ->
// NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_SSH_HOST).
func DeployEnvVar(environment, key string) string {
	return envPrefix(environment) + key
}

func envPrefix(environment string) string {
	return DeployEnvPrefix + "_" + strings.ToUpper(strings.ReplaceAll(environment, "-", "_")) + "_"
}

// applyDefaults fills every unset field with its documented default.
func (p *Pipeline) applyDefaults() {
	if p.Dotnet.Configuration == "" {
		p.Dotnet.Configuration = DefaultDotnetConfiguration
	}
	if p.Branches == nil {
		p.Branches = DefaultBranches()
	}
	for i := range p.Branches {
		if b := &p.Branches[i]; b.Channel == "" {
			b.Channel = b.Prerelease
		}
	}
	r := &p.Registry
	if r.Host == "" {
		r.Host = DefaultRegistryHost
	}
	if r.UsernameEnv == "" {
		r.UsernameEnv = DefaultRegistryUsernameEnv
	}
	if r.TokenEnv == "" {
		r.TokenEnv = DefaultRegistryTokenEnv
	}
	for i := range p.Images {
		img := &p.Images[i]
		img.Name = strings.ToLower(strings.TrimSpace(img.Name))
		if img.Title == "" && img.Name != "" {
			img.Title = path.Base(img.Name)
		}
		if img.Dockerfile == "" {
			img.Dockerfile = DefaultDockerfile
		}
		if img.Context == "" {
			img.Context = DefaultDockerContext
		}
		if img.Platform == "" {
			img.Platform = DefaultPlatform
		}
		if len(img.Tags) == 0 {
			img.Tags = shipconfig.DefaultDockerTags()
		}
		for name, arg := range img.ComputedArgs {
			if arg.Shell == "" {
				arg.Shell = DefaultShell
				img.ComputedArgs[name] = arg
			}
		}
	}
	if p.Sync.Develop == "" {
		p.Sync.Develop = DefaultSyncDevelop
	}
	if p.Sync.Release == "" {
		p.Sync.Release = DefaultSyncRelease
	}
	if d := p.Deploy; d != nil {
		if d.Source == "" {
			d.Source = DefaultDeploySource
		}
		if d.Environments == nil {
			d.Environments = DefaultEnvironments()
		}
		if d.SSH.Auth == "" {
			d.SSH.Auth = DefaultSSHAuth
		}
		if d.SSH.KnownHosts == "" {
			d.SSH.KnownHosts = DefaultSSHKnownHosts
		}
		if d.SSH.Port == 0 {
			d.SSH.Port = DefaultSSHPort
		}
		if d.SSH.ConnectTimeout == "" {
			d.SSH.ConnectTimeout = DefaultSSHConnectTimeout
		}
		if d.Verify.VersionLabel == "" {
			d.Verify.VersionLabel = DefaultVersionLabel
		}
		if d.Verify.Timeout == "" {
			d.Verify.Timeout = DefaultVerifyTimeout
		}
	}
}

// finalizePaths runs after validation and rewrites every path as a clean
// forward-slash path.
func (p *Pipeline) finalizePaths() {
	p.Dotnet.Solution = normalize(p.Dotnet.Solution)
	p.Dotnet.Runsettings = normalize(p.Dotnet.Runsettings)
	for i := range p.Images {
		p.Images[i].Dockerfile = normalize(p.Images[i].Dockerfile)
		p.Images[i].Context = normalize(p.Images[i].Context)
	}
}

func normalize(p string) string { return yamlsection.NormalizePath(p) }
