package releaseconfig

import (
	"path"
)

// The documented defaults of the release: section.
const (
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

// applyDefaults fills every unset field with its documented default.
func (r *Release) applyDefaults() {
	if r.TagFormat == "" {
		r.TagFormat = DefaultTagFormat
	}
	if r.Changelog.File == "" {
		r.Changelog.File = DefaultChangelog
	}
	single := len(r.Projects) == 1
	for i := range r.Projects {
		p := &r.Projects[i]
		if len(p.Scopes) == 0 && p.Name != "" {
			p.Scopes = []string{p.Name}
		}
		if p.TagFormat == "" {
			if single {
				p.TagFormat = r.TagFormat
			} else {
				p.TagFormat = p.Name + "-" + r.TagFormat
			}
		}
		for j := range p.DependsOn {
			if p.DependsOn[j].Bump == "" {
				p.DependsOn[j].Bump = DefaultBump
			}
		}
		if p.Build != nil {
			for j := range p.Build.Images {
				if img := &p.Build.Images[j]; img.Name != "" && len(img.Tags) == 0 {
					img.Tags = DefaultDockerTags()
				}
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
func (r *Release) defaultChangelog(projectPath string) string {
	return path.Join(projectPath, r.Changelog.File)
}
