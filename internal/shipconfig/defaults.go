package shipconfig

import (
	"path"
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
	// DefaultShell runs stage commands and hooks.
	DefaultShell = "bash"
)

// DefaultDockerTags returns the tags a nerv pipeline docker image receives
// when tags is not set: every tag kind (DockerTags). Each call returns a
// fresh slice, so a caller cannot change the defaults of later calls. Kept
// for nerv pipeline (internal/pipelineconfig) until it is folded into ship
// and deploy.
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
