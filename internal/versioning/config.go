package versioning

import (
	"fmt"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig/yamlsection"
	"gopkg.in/yaml.v3"
)

// Defaults of the engine's configuration.
const (
	// DefaultTagFormat is the tag of a release of a single project; with
	// several projects each one defaults to "<name>-" + the tag format.
	DefaultTagFormat = "v${version}"
	// DefaultChangelogFile is the CHANGELOG file name, under the project's
	// path.
	DefaultChangelogFile = "CHANGELOG.md"
	// DefaultBump is the bump a dependency's release gives its dependent.
	DefaultBump = "patch"
	// versionPlaceholder is the only placeholder of a tag format.
	versionPlaceholder = "${version}"
)

// Release types of a release rule. ReleaseNone stops a matching commit from
// releasing, as semantic-release's `release: false`.
const (
	ReleaseMajor = "major"
	ReleaseMinor = "minor"
	ReleasePatch = "patch"
	ReleaseNone  = "none"
)

// DefaultNoteKeywords are the breaking-change note keywords of
// dim.comprobantes.api's configuration (parserOpts.noteKeywords).
func DefaultNoteKeywords() []string {
	return []string{"BREAKING CHANGE", "BREAKING CHANGES", "BREAKING"}
}

// DefaultBranches are the release branches used when none is configured:
// main; develop (alpha); release/* (rc on the beta channel); hotfix/*
// (beta). Each call returns a fresh slice.
func DefaultBranches() []Branch {
	return []Branch{
		{Name: "main"},
		{Name: "develop", Prerelease: PrereleaseID("alpha"), Channel: "alpha"},
		{Name: "release/*", Prerelease: PrereleaseID("rc"), Channel: "beta"},
		{Name: "hotfix/*", Prerelease: PrereleaseID("beta"), Channel: "beta"},
	}
}

// Config is the version engine's configuration: what semantic-release reads
// from its own configuration, for the behaviours nerv relies on. The ship
// plugin's section of .nerv/nerv.yaml embeds it.
type Config struct {
	// Branches are the release branches, in semantic-release's order: a
	// branch matches the first entry whose name (or glob) matches it.
	// Default: DefaultBranches.
	Branches []Branch `yaml:"branches"`
	// ReleaseRules override commit-analyzer's default rules the way its
	// releaseRules option does: a commit matching no rule falls back to the
	// defaults. Default: none.
	ReleaseRules []ReleaseRule `yaml:"release_rules"`
	// NoteKeywords start a breaking-change note in a commit's body or
	// footer (parserOpts.noteKeywords). Default: DefaultNoteKeywords.
	NoteKeywords []string `yaml:"note_keywords"`
	// TagFormat is the tag of a release, with ${version}. Default:
	// DefaultTagFormat.
	TagFormat string `yaml:"tag_format"`
	// RepositoryURL is the repository address the release notes link to.
	// Default: origin's URL, without credentials; a local origin (or none)
	// gives notes without links.
	RepositoryURL string `yaml:"repository_url"`
	// Changelog is the CHANGELOG file and title (@semantic-release/changelog).
	Changelog Changelog `yaml:"changelog"`
	// Projects are the independently versioned units of a monorepo. Empty
	// means one project: the whole repository.
	Projects []Project `yaml:"projects"`
}

// Changelog configures the CHANGELOG file.
type Changelog struct {
	// File is the CHANGELOG file name, under each project's path. Default:
	// DefaultChangelogFile.
	File string `yaml:"file"`
	// Title is written at the top of the file (changelogTitle). Default:
	// none.
	Title string `yaml:"title"`
}

// Branch is one release branch entry.
type Branch struct {
	// Name is a branch name or a glob (release/*). Required.
	Name string `yaml:"name"`
	// Prerelease makes the branch a prerelease branch: an identifier (rc),
	// or true to use the branch name.
	Prerelease Prerelease `yaml:"prerelease"`
	// Channel is the distribution channel. Default: semantic-release's,
	// the default channel for the first release branch and the branch name
	// for the others.
	Channel string `yaml:"channel"`
}

// UnmarshalYAML accepts a bare branch name as well as a mapping.
func (b *Branch) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&b.Name)
	}
	type plain Branch
	var p plain
	if err := node.Decode(&p); err != nil {
		return err
	}
	*b = Branch(p)
	return nil
}

// Prerelease is a branch's prerelease setting: unset, an identifier, or
// true (the branch name).
type Prerelease struct {
	id      string
	useName bool
}

// PrereleaseID is a fixed prerelease identifier.
func PrereleaseID(id string) Prerelease { return Prerelease{id: id} }

// PrereleaseFromName uses the branch name as the identifier
// (`prerelease: true`).
func PrereleaseFromName() Prerelease { return Prerelease{useName: true} }

// IsSet reports whether the branch is a prerelease branch.
func (p Prerelease) IsSet() bool { return p.useName || p.id != "" }

// UsesName reports whether the identifier is the branch name.
func (p Prerelease) UsesName() bool { return p.useName }

// ID is the fixed identifier ("" for unset or true).
func (p Prerelease) ID() string { return p.id }

// resolve returns the identifier for the branch named name.
func (p Prerelease) resolve(name string) string {
	if p.useName {
		return name
	}
	return p.id
}

// UnmarshalYAML reads true, false or an identifier.
func (p *Prerelease) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.ShortTag() == "!!bool" {
		var b bool
		if err := node.Decode(&b); err != nil {
			return err
		}
		*p = Prerelease{useName: b}
		return nil
	}
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	*p = Prerelease{id: s}
	return nil
}

// MarshalYAML writes true or the identifier.
func (p Prerelease) MarshalYAML() (any, error) {
	if p.useName {
		return true, nil
	}
	return p.id, nil
}

// IsZero lets omitempty drop an unset prerelease.
func (p Prerelease) IsZero() bool { return !p.IsSet() }

// ReleaseRule is one commit-analyzer release rule.
type ReleaseRule struct {
	// Type matches the commit type (a glob). Empty matches any.
	Type string `yaml:"type"`
	// Scope matches the commit scope (a glob). Empty matches any.
	Scope string `yaml:"scope"`
	// Breaking matches only commits with a breaking-change note.
	Breaking bool `yaml:"breaking"`
	// Revert matches only revert commits.
	Revert bool `yaml:"revert"`
	// Release is major, minor, patch or none (false is read as none).
	// Required.
	Release string `yaml:"release"`
}

// UnmarshalYAML reads `release: false` as ReleaseNone.
func (r *ReleaseRule) UnmarshalYAML(node *yaml.Node) error {
	type plain struct {
		Type     string    `yaml:"type"`
		Scope    string    `yaml:"scope"`
		Breaking bool      `yaml:"breaking"`
		Revert   bool      `yaml:"revert"`
		Release  yaml.Node `yaml:"release"`
	}
	var p plain
	if err := node.Decode(&p); err != nil {
		return err
	}
	*r = ReleaseRule{Type: p.Type, Scope: p.Scope, Breaking: p.Breaking, Revert: p.Revert}
	switch {
	case p.Release.Kind == 0:
	case p.Release.ShortTag() == "!!bool":
		var b bool
		if err := p.Release.Decode(&b); err != nil {
			return err
		}
		if b {
			return fmt.Errorf("line %d: release: true is not a release type", p.Release.Line)
		}
		r.Release = ReleaseNone
	case p.Release.ShortTag() == "!!null":
		r.Release = ReleaseNone
	default:
		if err := p.Release.Decode(&r.Release); err != nil {
			return err
		}
	}
	return nil
}

// Project is one independently versioned unit of the repository.
type Project struct {
	// Name identifies the project: unique, [a-z0-9][a-z0-9-]*. Required.
	Name string `yaml:"name"`
	// Path is the project's directory; "." is the whole repository.
	// Required.
	Path string `yaml:"path"`
	// SharedPaths are extra paths or globs whose changes count for the
	// project.
	SharedPaths []string `yaml:"shared_paths"`
	// Scopes are the conventional-commit scopes whose commits count for the
	// project wherever they touch. Default: [Name].
	Scopes []string `yaml:"scopes"`
	// TagFormat is the project's tag, with ${version}. Default: the
	// configuration's tag format with one project, "<name>-" + it
	// otherwise.
	TagFormat string `yaml:"tag_format"`
	// Changelog is the project's CHANGELOG file. Default: <path>/<file>.
	Changelog string `yaml:"changelog"`
	// DependsOn lists the projects whose new release bumps this one.
	DependsOn []Dependency `yaml:"depends_on"`
}

// Dependency makes a project follow another project's releases.
type Dependency struct {
	// Project names another project. Required.
	Project string `yaml:"project"`
	// Bump is the lowest release the dependent gets when the dependency
	// releases: patch, minor, major or none. Default: DefaultBump.
	Bump string `yaml:"bump"`
}

// ParseConfig reads a configuration from YAML (or JSON), refusing unknown
// keys, and returns it with its defaults applied and validated.
func ParseConfig(data []byte) (Config, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Config{}, fmt.Errorf("version configuration: %w", err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return Config{}.WithDefaults(), nil
	}
	return DecodeConfig(doc.Content[0], "versioning")
}

// DecodeConfig reads a configuration from a YAML node (the ship plugin's
// section embeds one), refusing unknown keys reported under root, and
// returns it with its defaults applied and validated.
func DecodeConfig(node *yaml.Node, root string) (Config, error) {
	unknown, err := yamlsection.UnknownFields(node, reflect.TypeOf(Config{}), root)
	if err != nil {
		return Config{}, err
	}
	if len(unknown) > 0 {
		return Config{}, fmt.Errorf("version configuration: %s", strings.Join(unknown, "; "))
	}
	var c Config
	if err := node.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("version configuration: %w", err)
	}
	c = c.WithDefaults()
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// WithDefaults returns a copy of c with every unset setting defaulted.
func (c Config) WithDefaults() Config {
	out := c
	if len(out.Branches) == 0 {
		out.Branches = DefaultBranches()
	} else {
		out.Branches = slices.Clone(out.Branches)
	}
	if len(out.NoteKeywords) == 0 {
		out.NoteKeywords = DefaultNoteKeywords()
	}
	if out.TagFormat == "" {
		out.TagFormat = DefaultTagFormat
	}
	if out.Changelog.File == "" {
		out.Changelog.File = DefaultChangelogFile
	}
	out.ReleaseRules = slices.Clone(out.ReleaseRules)
	single := len(out.Projects) <= 1
	projects := make([]Project, len(out.Projects))
	for i, p := range out.Projects {
		p.Path = yamlsection.NormalizePath(p.Path)
		if len(p.Scopes) == 0 && p.Name != "" {
			p.Scopes = []string{p.Name}
		}
		if p.TagFormat == "" {
			if single {
				p.TagFormat = out.TagFormat
			} else {
				p.TagFormat = p.Name + "-" + out.TagFormat
			}
		}
		if p.Changelog == "" {
			p.Changelog = path.Join(p.Path, out.Changelog.File)
		} else {
			p.Changelog = yamlsection.NormalizePath(p.Changelog)
		}
		deps := make([]Dependency, len(p.DependsOn))
		for j, d := range p.DependsOn {
			if d.Bump == "" {
				d.Bump = DefaultBump
			}
			deps[j] = d
		}
		p.DependsOn = deps
		p.SharedPaths = slices.Clone(p.SharedPaths)
		projects[i] = p
	}
	out.Projects = projects
	return out
}

var projectNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Validate reports every problem of a defaulted configuration at once.
func (c Config) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	for i, b := range c.Branches {
		if strings.TrimSpace(b.Name) == "" {
			add("branches[%d]: name is required", i)
		}
	}
	for i, r := range c.ReleaseRules {
		switch r.Release {
		case ReleaseMajor, ReleaseMinor, ReleasePatch, ReleaseNone:
		case "":
			add("release_rules[%d]: release is required (major, minor, patch or none)", i)
		default:
			add("release_rules[%d]: release %q is not major, minor, patch or none", i, r.Release)
		}
	}
	for i, k := range c.NoteKeywords {
		if strings.TrimSpace(k) == "" {
			add("note_keywords[%d]: empty keyword", i)
		}
	}
	if p := tagFormatProblem(c.TagFormat); p != "" {
		add("tag_format: %s", p)
	}

	names := map[string]bool{}
	tags := map[string]string{}
	for i, p := range c.Projects {
		at := fmt.Sprintf("projects[%d]", i)
		switch {
		case p.Name == "":
			add("%s: name is required", at)
		case !projectNameRe.MatchString(p.Name):
			add("%s: name %q must match [a-z0-9][a-z0-9-]*", at, p.Name)
		case names[p.Name]:
			add("%s: duplicate project name %q", at, p.Name)
		}
		names[p.Name] = true
		if p.Path == "" {
			add("%s.path: required (\".\" is the whole repository)", at)
		} else if pp := yamlsection.PathProblem(at+".path", p.Path); pp != "" {
			add("%s", pp)
		}
		for j, s := range p.SharedPaths {
			if pp := yamlsection.PathProblem(fmt.Sprintf("%s.shared_paths[%d]", at, j), yamlsection.NormalizePath(s)); pp != "" {
				add("%s", pp)
			}
		}
		if pp := yamlsection.PathProblem(at+".changelog", p.Changelog); pp != "" {
			add("%s", pp)
		}
		if tp := tagFormatProblem(p.TagFormat); tp != "" {
			add("%s.tag_format: %s", at, tp)
		} else if other, dup := tags[p.TagFormat]; dup {
			add("%s.tag_format: %q is also the tag format of project %q", at, p.TagFormat, other)
		}
		tags[p.TagFormat] = p.Name
		for j, d := range p.DependsOn {
			switch d.Bump {
			case ReleasePatch, ReleaseMinor, ReleaseMajor, ReleaseNone:
			default:
				add("%s.depends_on[%d].bump: %q is not patch, minor, major or none", at, j, d.Bump)
			}
		}
	}
	for i, p := range c.Projects {
		for j, d := range p.DependsOn {
			if !names[d.Project] {
				add("projects[%d].depends_on[%d]: unknown project %q", i, j, d.Project)
			} else if d.Project == p.Name {
				add("projects[%d].depends_on[%d]: a project cannot depend on itself", i, j)
			}
		}
	}
	if len(problems) == 0 {
		if _, err := topoOrder(c.Projects); err != nil {
			add("projects: %v", err)
		}
	}
	if len(problems) > 0 {
		return &ConfigError{Problems: problems}
	}
	return nil
}

// ConfigError lists the problems of a configuration.
type ConfigError struct{ Problems []string }

func (e *ConfigError) Error() string {
	return "version configuration: " + strings.Join(e.Problems, "; ")
}

// tagFormatProblem checks a tag format: ${version} exactly once, no other
// placeholder.
func tagFormatProblem(format string) string {
	if strings.Count(format, versionPlaceholder) != 1 {
		return fmt.Sprintf("%q must contain %s exactly once", format, versionPlaceholder)
	}
	if strings.Contains(strings.Replace(format, versionPlaceholder, "", 1), "${") {
		return fmt.Sprintf("%q has a placeholder other than %s", format, versionPlaceholder)
	}
	return ""
}

// topoOrder returns the projects with every project after the projects it
// depends on, otherwise in declaration order; it fails on a cycle.
func topoOrder(projects []Project) ([]Project, error) {
	index := make(map[string]int, len(projects))
	for i, p := range projects {
		index[p.Name] = i
	}
	const (
		unvisited = iota
		visiting
		done
	)
	state := make([]int, len(projects))
	order := make([]Project, 0, len(projects))
	var stack []string
	var visit func(i int) error
	visit = func(i int) error {
		p := projects[i]
		switch state[i] {
		case done:
			return nil
		case visiting:
			start := slices.Index(stack, p.Name)
			cycle := append(slices.Clone(stack[start:]), p.Name)
			return fmt.Errorf("dependency cycle %s", strings.Join(cycle, " -> "))
		}
		state[i] = visiting
		stack = append(stack, p.Name)
		for _, d := range p.DependsOn {
			j, ok := index[d.Project]
			if !ok {
				return fmt.Errorf("project %q depends on unknown project %q", p.Name, d.Project)
			}
			if err := visit(j); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[i] = done
		order = append(order, p)
		return nil
	}
	for i := range projects {
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	return order, nil
}
