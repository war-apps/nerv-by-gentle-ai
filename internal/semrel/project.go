package semrel

import (
	"errors"
	"fmt"
	"slices"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// project is one semantic-release computation.
type project struct {
	Name string
	// TagFormat overrides the repository's tagFormat; empty keeps it.
	TagFormat string `json:",omitempty"`
	// Filter selects the project's commits; nil means every commit.
	Filter    *commitFilter           `json:",omitempty"`
	DependsOn []shipconfig.Dependency `json:",omitempty"`
}

// planProjects lists the computations for a release section, in dependency
// order. With no section (rel nil) the whole repository is one project named
// repoName that behaves exactly like plain semantic-release. A single project
// with the default tag format keeps the repository's own tagFormat, and a
// project whose path is the repository root sees every commit. only selects
// one project, preceded by the projects it depends on (directly or not) so
// their releases can bump it.
func planProjects(rel *shipconfig.Release, repoName, only string) ([]project, error) {
	if rel == nil {
		if only != "" {
			return nil, fmt.Errorf("project %q: the repository has no release: section in .nerv/nerv.yaml", only)
		}
		return []project{{Name: repoName}}, nil
	}
	ordered, err := rel.TopoOrder()
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	if only != "" {
		if _, ok := rel.Project(only); !ok {
			return nil, fmt.Errorf("unknown project %q", only)
		}
		var mark func(name string)
		mark = func(name string) {
			if keep[name] {
				return
			}
			keep[name] = true
			p, _ := rel.Project(name)
			for _, d := range p.DependsOn {
				mark(d.Project)
			}
		}
		mark(only)
	}

	single := len(rel.Projects) == 1
	var out []project
	for _, p := range ordered {
		if only != "" && !keep[p.Name] {
			continue
		}
		pr := project{Name: p.Name, TagFormat: p.TagFormat, DependsOn: p.DependsOn}
		if single && p.TagFormat == shipconfig.DefaultTagFormat {
			pr.TagFormat = ""
		}
		if p.Path != "." {
			pr.Filter = &commitFilter{
				Paths:  append([]string{p.Path}, p.SharedPaths...),
				Scopes: slices.Clone(p.Scopes),
			}
		}
		out = append(out, pr)
	}
	if len(out) == 0 {
		return nil, errors.New("no project to compute")
	}
	return out, nil
}
