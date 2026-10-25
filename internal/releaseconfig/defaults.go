package releaseconfig

// The documented defaults of the release: section. A project's tag format
// and changelog default to the version engine's (internal/releaseplan
// gives the engine the projects as written).
const (
	// DefaultBump is the bump a dependency's release applies to its dependent.
	DefaultBump = "patch"
	// DefaultShell runs stage commands and hooks.
	DefaultShell = "bash"
)

// applyDefaults fills every unset field with its documented default.
func (r *Release) applyDefaults() {
	for i := range r.Projects {
		p := &r.Projects[i]
		if len(p.Scopes) == 0 && p.Name != "" {
			p.Scopes = []string{p.Name}
		}
		for j := range p.DependsOn {
			if p.DependsOn[j].Bump == "" {
				p.DependsOn[j].Bump = DefaultBump
			}
		}
		for j := range p.Images {
			if img := &p.Images[j]; img.Name != "" && len(img.Tags) == 0 {
				img.Tags = DefaultDockerTags()
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
