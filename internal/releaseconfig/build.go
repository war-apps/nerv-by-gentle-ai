package releaseconfig

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// VersionFileExtensions are the MSBuild project files version_files may
// name: nerv release writes <Version> (and <PackageReleaseNotes>) in them.
var VersionFileExtensions = []string{".csproj", ".fsproj", ".vbproj", ".props", ".targets"}

// DockerTagKinds are the tag kinds an images mapping may list, as
// 4.2.0's docker preset tagged its images: version always; channel only
// off the default channel; latest only on the default channel.
var DockerTagKinds = []string{"version", "channel", "latest"}

// DefaultDockerTags returns the tag kinds of an images mapping
// without tags: every kind (DockerTagKinds), as in 4.2.0. Each call
// returns a fresh slice, so a caller cannot change later defaults.
func DefaultDockerTags() []string { return slices.Clone(DockerTagKinds) }

// BuildImage is one images entry: either a plain image reference
// (Ref, the string form) or a repository with tag kinds (Name and Tags,
// the mapping form {name, tags}).
type BuildImage struct {
	// Ref is the plain-string form: an image reference that may use
	// ${version}, ${channel} and ${tag}.
	Ref string `yaml:"-"`
	// Name is the mapping form's repository, without a tag or digest.
	Name string `yaml:"name"`
	// Tags are the mapping form's tag kinds (DockerTagKinds). Default:
	// DefaultDockerTags.
	Tags []string `yaml:"tags"`
}

// UnmarshalYAML decodes a plain reference or a {name, tags} mapping.
func (i *BuildImage) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	switch n.Kind {
	case yaml.ScalarNode:
		*i = BuildImage{}
		return n.Decode(&i.Ref)
	case yaml.MappingNode:
		var m struct {
			Name string   `yaml:"name"`
			Tags []string `yaml:"tags"`
		}
		if err := n.Decode(&m); err != nil {
			return err
		}
		*i = BuildImage{Name: m.Name, Tags: m.Tags}
		return nil
	}
	return fmt.Errorf("line %d: an images entry is an image reference or a mapping {name, tags}", n.Line)
}

// Coverage is a project's coverage.
type Coverage struct {
	// Dir is the directory searched for *.cobertura.xml reports, relative
	// to the repository root. Default: empty, meaning
	// $NERV_RELEASE_ARTIFACTS_DIR/coverage.
	Dir    string  `yaml:"dir"`
	Line   float64 `yaml:"line"`
	Branch float64 `yaml:"branch"`
	Method float64 `yaml:"method"`
}

// Floors returns the coverage floors.
func (c Coverage) Floors() CoverageThreshold {
	return CoverageThreshold{Line: c.Line, Branch: c.Branch, Method: c.Method}
}

// CheckBuild is nerv release's requirement on top of Load: at least one
// project, and every project has a build provider (its build, or the
// section's). (nerv version next needs neither: a section without projects
// versions the whole repository.) It returns a *ValidationError naming
// each problem.
func (r *Release) CheckBuild() error {
	var problems []string
	if len(r.Projects) == 0 {
		problems = append(problems, "release.projects: nerv release needs at least one project, each with its build provider (release.build, or release.projects[].build: docker-compose or bash)")
	}
	for i, p := range r.Projects {
		if r.BuildProvider(p) == "" {
			problems = append(problems, fmt.Sprintf("release.projects[%d].build: required by nerv release (the build provider, such as docker-compose or bash; or set release.build for every project): there is no default", i))
		}
	}
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

var imageRe = regexp.MustCompile(`^[^\s]+$`)

// checkBuild validates the build keys of project p: its build provider's
// name, images, version_files and coverage (Load: a missing build is
// fine, CheckBuild requires one; the provider checks its own keys).
func (v *validator) checkBuild(at string, p Project) {
	if p.Build != "" && !nameRe.MatchString(p.Build) {
		v.addf("%s.build: %q is not a provider name", at, p.Build)
	}
	for j, img := range p.Images {
		v.checkImage(fmt.Sprintf("%s.images[%d]", at, j), img)
	}
	for j, f := range p.VersionFiles {
		fat := fmt.Sprintf("%s.version_files[%d]", at, j)
		if problem := v.pathProblem(fat, f); problem {
			continue
		}
		if !slices.Contains(VersionFileExtensions, strings.ToLower(path.Ext(normalize(f)))) {
			v.addf("%s: %q is not an MSBuild project file (%s)", fat, f, list(VersionFileExtensions))
		}
	}
	if c := p.Coverage; c != nil {
		cat := at + ".coverage"
		if c.Dir != "" {
			v.checkPath(cat+".dir", c.Dir)
		}
		v.checkPercent(cat+".line", c.Line)
		v.checkPercent(cat+".branch", c.Branch)
		v.checkPercent(cat+".method", c.Method)
		if !c.Floors().Enabled() {
			v.addf("%s: set at least one floor (line, branch or method)", cat)
		}
	}
}

// checkImage validates one images entry.
func (v *validator) checkImage(at string, img BuildImage) {
	if img.Ref == "" && (img.Name != "" || img.Tags != nil) {
		v.checkImageMapping(at, img)
		return
	}
	switch {
	case strings.TrimSpace(img.Ref) == "":
		v.addf("%s: required", at)
	case !imageRe.MatchString(img.Ref):
		v.addf("%s: %q is not an image reference", at, img.Ref)
	}
	for _, m := range placeholderRe.FindAllStringSubmatch(img.Ref, -1) {
		if !slices.Contains(BuildArgPlaceholders, m[1]) {
			v.addf("%s: unknown placeholder ${%s} (want %s)", at, m[1], list(placeholders(BuildArgPlaceholders)))
		}
	}
}

// repositoryRe is a repository without a tag or digest: an optional
// registry host (which may carry a :port) and lower-case path components.
var repositoryRe = regexp.MustCompile(`^([A-Za-z0-9.-]+(:[0-9]+)?/)?[a-z0-9]+([._-]+[a-z0-9]+)*(/[a-z0-9]+([._-]+[a-z0-9]+)*)*$`)

// checkImageMapping validates the {name, tags} form.
func (v *validator) checkImageMapping(at string, img BuildImage) {
	switch {
	case img.Name == "":
		v.addf("%s.name: required", at)
	case !repositoryRe.MatchString(img.Name):
		v.addf("%s.name: %q is not a repository (no tag, digest or placeholder: tags gives the tags)", at, img.Name)
	}
	for k, kind := range img.Tags {
		if !slices.Contains(DockerTagKinds, kind) {
			v.addf("%s.tags[%d]: unknown tag kind %q (want %s)", at, k, kind, list(DockerTagKinds))
		}
	}
}

// pathProblem is checkPath reporting whether it found a problem.
func (v *validator) pathProblem(at, raw string) bool {
	n := len(v.problems)
	v.checkPath(at, raw)
	return len(v.problems) > n
}

// removedKey is a project key nerv release no longer reads, with what to
// write instead.
type removedKey struct {
	key, message string
}

// removedProjectKeys are the preset keys removed with the presets: Load
// reports each with its replacement instead of an unknown field.
var removedProjectKeys = []removedKey{
	{"presets", `the presets were removed; set the build provider instead: "build: bash" with each stage's command under stages (e.g. ` +
		`"stages: { build: { run: "dotnet build -c Release" } }"), or ` +
		`"build: docker-compose" with "providers: { docker-compose: { file: docker-compose.yml, test_service: tests } }" and "images: ["ghcr.io/org/api:${version}"]"`},
	{"dotnet", `the dotnet preset was removed; with build: bash write its commands as stages, e.g. ` +
		`"stages: { restore: { run: "dotnet restore" }, lint: { run: "dotnet format --verify-no-changes" }, build: { run: "dotnet build -c Release --no-restore" }, test: { run: "dotnet test -c Release --no-build" } }"; ` +
		`project_file goes to version_files, coverage_threshold to coverage (line, branch, method; the tests write *.cobertura.xml under coverage.dir), ` +
		`shared_host to providers.bash.shared_host, and for sdk: write global.json in a before_restore hook`},
	{"node", `the node preset was removed; with build: bash write its commands as stages, e.g. ` +
		`"stages: { restore: { run: "npm ci" }, lint: { run: "npm run lint" }, build: { run: "npm run build" }, test: { run: "npm test" } }"`},
	{"docker", `the docker preset was removed; use ` +
		`"build: docker-compose" with "providers: { docker-compose: { file: docker-compose.yml } }" and "images: ["ghcr.io/org/api:${version}"]", or with build: bash a package stage such as ` +
		`"stages: { package: { run: "docker build -t ghcr.io/org/api:$NERV_RELEASE_VERSION . && docker save -o \"$NERV_RELEASE_OUTPUTS_DIR/api.tar\" ghcr.io/org/api:$NERV_RELEASE_VERSION" } }"; ` +
		`pushing moved to nerv deploy`},
	{"nuget", `the nuget preset was removed; package in a bash stage instead: ` +
		`"stages: { package: { run: "dotnet pack -c Release --no-build -o \"$NERV_RELEASE_OUTPUTS_DIR\"" } }"; pushing moved to nerv deploy`},
	{"npm", `the npm preset was removed; package in a bash stage instead: ` +
		`"stages: { package: { run: "npm pack --pack-destination \"$NERV_RELEASE_OUTPUTS_DIR\"" } }"; publishing moved to nerv deploy`},
}

// removedStages and removedHooks are the stage and hook keys removed from
// nerv release, with what to do instead.
var (
	removedStages = map[string]string{
		"publish": "the publish stage was removed from nerv release (publishing moved to nerv deploy); write the outputs to $NERV_RELEASE_OUTPUTS_DIR in the package stage instead",
	}
	removedHooks = map[string]string{
		"publish": "the publish hooks were removed from nerv release: they belong to nerv deploy now",
		"success": "the success hooks were removed from nerv release: they belong to nerv deploy now",
		// There is no publish stage to hook before or after.
		"before_publish": "the publish stage moved to nerv deploy (the deploy: section's targets), so there is nothing to hook: run it from a deploy target, or use the prepare hook to act before the artifact is written",
		"after_publish":  "the publish stage moved to nerv deploy (the deploy: section's targets), so there is nothing to hook: run it from a deploy target after the push",
	}
)

// takeRemovedKeys reports a 4.2.0 section (legacyMessage) and every removed
// key of the release: node and of its projects, and deletes each from the
// node, so it is neither decoded nor
// reported as an unknown field.
func takeRemovedKeys(release *yaml.Node) []string {
	var problems []string
	if msg := legacyMessage(release); msg != "" {
		problems = append(problems, msg)
	}
	problems = append(problems, takeRemovedSectionKeys(release)...)
	problems = append(problems, takeMovedKeys(release)...)
	projects := mappingValue(release, "projects")
	if projects == nil || projects.Kind != yaml.SequenceNode {
		return problems
	}
	for i, p := range projects.Content {
		if p.Kind != yaml.MappingNode {
			continue
		}
		kept := p.Content[:0:0]
		for j := 0; j+1 < len(p.Content); j += 2 {
			key := p.Content[j].Value
			idx := slices.IndexFunc(removedProjectKeys, func(r removedKey) bool { return r.key == key })
			if idx < 0 {
				kept = append(kept, p.Content[j], p.Content[j+1])
				continue
			}
			problems = append(problems, fmt.Sprintf("release.projects[%d].%s: %s", i, key, removedProjectKeys[idx].message))
		}
		p.Content = kept
	}
	return problems
}

// mappingValue is the value of key in mapping node m, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for j := 0; j+1 < len(m.Content); j += 2 {
		if m.Content[j].Value == key {
			return m.Content[j+1]
		}
	}
	return nil
}
