package shipconfig

import (
	"path"
	"slices"
)

// The documented defaults of the release: section (docs/release-commands.md).
const (
	// DefaultSemanticReleaseVersion is the semantic-release version installed
	// in the NERV cache when release.semantic_release.version is not set.
	DefaultSemanticReleaseVersion = "24.2.3"
	// DefaultTagFormat is the tag format of a single-project section; with
	// several projects each one defaults to "<name>-" + DefaultTagFormat.
	DefaultTagFormat = "v${version}"
	// DefaultChangelog is the CHANGELOG file name, under the project's path.
	DefaultChangelog = "CHANGELOG.md"
	// DefaultBump is the bump a dependency's release applies to its dependent.
	DefaultBump = "patch"
	// DefaultDotnetConfiguration is the dotnet build configuration.
	DefaultDotnetConfiguration = "Release"
	// DefaultPackageManager is the node package manager.
	DefaultPackageManager = "npm"
	// DefaultDockerfile is the Dockerfile path of a docker image.
	DefaultDockerfile = "Dockerfile"
	// DefaultDockerContext is the build context of a docker image.
	DefaultDockerContext = "."
	// DefaultNuGetSource is the nuget push source.
	DefaultNuGetSource = "https://api.nuget.org/v3/index.json"
	// DefaultNuGetAPIKeyEnv is the environment variable holding the NuGet
	// API key.
	DefaultNuGetAPIKeyEnv = "NUGET_API_KEY"
	// DefaultNpmAccess is the npm publish access level.
	DefaultNpmAccess = "public"
	// DefaultShell runs stage overrides and hooks.
	DefaultShell = "bash"
)

// DefaultDockerTags returns the tags a docker image receives when tags is
// not set: every tag kind (DockerTags). Each call returns a fresh slice, so
// a caller cannot change the defaults of later loads.
func DefaultDockerTags() []string {
	return []string{"version", "channel", "latest"}
}

// applyDefaults fills every unset field with its documented default.
func (r *Release) applyDefaults() {
	if r.SemanticRelease.Version == "" {
		r.SemanticRelease.Version = DefaultSemanticReleaseVersion
	}
	single := len(r.Projects) == 1
	for i := range r.Projects {
		p := &r.Projects[i]
		if len(p.Scopes) == 0 && p.Name != "" {
			p.Scopes = []string{p.Name}
		}
		if p.TagFormat == "" {
			if single {
				p.TagFormat = DefaultTagFormat
			} else {
				p.TagFormat = p.Name + "-" + DefaultTagFormat
			}
		}
		for j := range p.DependsOn {
			if p.DependsOn[j].Bump == "" {
				p.DependsOn[j].Bump = DefaultBump
			}
		}
		p.applyPresetDefaults()
		for stage, c := range p.Stages {
			p.Stages[stage] = c.withDefaults()
		}
		for key, cmds := range p.Hooks {
			for j := range cmds {
				cmds[j] = cmds[j].withDefaults()
			}
			p.Hooks[key] = cmds
		}
	}
}

// applyPresetDefaults creates the settings of every enabled preset and
// fills their defaults.
func (p *Project) applyPresetDefaults() {
	enabled := func(name string) bool { return slices.Contains(p.Presets, name) }
	if enabled("dotnet") && p.Dotnet == nil {
		p.Dotnet = &Dotnet{}
	}
	if p.Dotnet != nil && p.Dotnet.Configuration == "" {
		p.Dotnet.Configuration = DefaultDotnetConfiguration
	}
	if enabled("node") && p.Node == nil {
		p.Node = &Node{}
	}
	if p.Node != nil && p.Node.PackageManager == "" {
		p.Node.PackageManager = DefaultPackageManager
	}
	if enabled("docker") && p.Docker == nil {
		p.Docker = &Docker{}
	}
	if p.Docker != nil {
		for i := range p.Docker.Images {
			img := &p.Docker.Images[i]
			if img.Dockerfile == "" {
				img.Dockerfile = DefaultDockerfile
			}
			if img.Context == "" {
				img.Context = DefaultDockerContext
			}
			if len(img.Tags) == 0 {
				img.Tags = DefaultDockerTags()
			}
			if img.Push == nil {
				push := true
				img.Push = &push
			}
		}
	}
	if enabled("nuget") && p.NuGet == nil {
		p.NuGet = &NuGet{}
	}
	if p.NuGet != nil {
		if p.NuGet.Source == "" {
			p.NuGet.Source = DefaultNuGetSource
		}
		if p.NuGet.APIKeyEnv == "" {
			p.NuGet.APIKeyEnv = DefaultNuGetAPIKeyEnv
		}
	}
	if enabled("npm") && p.Npm == nil {
		p.Npm = &Npm{}
	}
	if p.Npm != nil && p.Npm.Access == "" {
		p.Npm.Access = DefaultNpmAccess
	}
}

func (c Command) withDefaults() Command {
	if c.Shell == "" {
		c.Shell = DefaultShell
	}
	return c
}

// defaultChangelog is the CHANGELOG of a project at projectPath.
func defaultChangelog(projectPath string) string {
	return path.Join(projectPath, DefaultChangelog)
}
