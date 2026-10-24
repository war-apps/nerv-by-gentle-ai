package shipconfig

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// The build modes.
const (
	// ModeCompose builds with docker compose build, tests with docker
	// compose run --rm <test_service> and packages the images with docker
	// save.
	ModeCompose = "compose"
	// ModeScript runs the project's own stage commands.
	ModeScript = "script"
)

// Modes are the build modes.
var Modes = []string{ModeCompose, ModeScript}

// ComposeStages are the stages a compose-mode project may still write
// commands for: compose runs build, test and package itself.
var ComposeStages = []string{"restore", "lint", "coverage"}

// VersionFileExtensions are the MSBuild project files build.version_files
// may name: nerv ship writes <Version> (and <PackageReleaseNotes>) in them.
var VersionFileExtensions = []string{".csproj", ".fsproj", ".vbproj", ".props", ".targets"}

// Build is a project's build: section.
type Build struct {
	// Mode is compose or script. Required by nerv ship; no default.
	Mode string `yaml:"mode"`
	// File is the compose file (compose mode, required).
	File string `yaml:"file"`
	// TestService is the compose service whose exit code is the test
	// result (compose mode). Default: empty, meaning no test stage.
	TestService string `yaml:"test_service"`
	// Images are the image references docker save packages into the
	// artifact after the package stage, recorded in its manifest so
	// deploy can push them; they may use ${version}, ${channel} and
	// ${tag}. Compose mode builds them with docker compose build; in
	// script mode the project's own commands (the package stage, usually)
	// must build them.
	Images []string `yaml:"images"`
	// SharedHost gives the restore, lint, build, test and package stage
	// commands the build-server-safe dotnet environment under their own env
	// (script mode). Default: false.
	SharedHost bool `yaml:"shared_host"`
	// VersionFiles are MSBuild project files that receive the version in
	// the release clone before the pipeline.
	VersionFiles []string `yaml:"version_files"`
	// Coverage checks coverage floors over Cobertura reports after the
	// coverage stage.
	Coverage *Coverage `yaml:"coverage"`
}

// Coverage is build.coverage.
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

// CheckBuild is nerv ship's requirement on top of Load: at least one
// project, and every project has a build mode. (nerv version next needs
// neither: a section without projects versions the whole repository.) It
// returns a *ValidationError naming each problem.
func (r *Release) CheckBuild() error {
	var problems []string
	if len(r.Projects) == 0 {
		problems = append(problems, "ship.projects: nerv ship needs at least one project, each with its build.mode (compose or script)")
	}
	for i, p := range r.Projects {
		if p.Build == nil || p.Build.Mode == "" {
			problems = append(problems, fmt.Sprintf("ship.projects[%d].build.mode: required by nerv ship (compose or script): there is no default", i))
		}
	}
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

var (
	serviceRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	imageRe   = regexp.MustCompile(`^[^\s]+$`)
)

// checkBuild validates the build: block of p (Load: a missing block is
// fine, CheckBuild requires it).
func (v *validator) checkBuild(at string, p Project) {
	b := p.Build
	if b == nil {
		return
	}
	bat := at + ".build"
	switch {
	case b.Mode == "":
		v.addf("%s.mode: required (compose or script)", bat)
	case !slices.Contains(Modes, b.Mode):
		v.addf("%s.mode: unknown mode %q (want %s)", bat, b.Mode, list(Modes))
	}
	if b.Mode == ModeCompose {
		if b.File == "" {
			v.addf("%s.file: required by compose mode", bat)
		} else {
			v.checkPath(bat+".file", b.File)
		}
		if b.TestService != "" && !serviceRe.MatchString(b.TestService) {
			v.addf("%s.test_service: %q is not a compose service name", bat, b.TestService)
		}
		for _, stage := range sortedKeys(p.Stages) {
			if slices.Contains(Stages, stage) && !slices.Contains(ComposeStages, stage) {
				v.addf("%s.stages.%s: compose mode runs %s itself (docker compose build, run and save); remove it, or use build.mode: script", at, stage, stage)
			}
		}
		if b.SharedHost {
			v.addf("%s.shared_host: only for script mode", bat)
		}
	} else if b.Mode == ModeScript {
		if b.File != "" {
			v.addf("%s.file: only for compose mode", bat)
		}
		if b.TestService != "" {
			v.addf("%s.test_service: only for compose mode", bat)
		}
	}
	for j, img := range b.Images {
		iat := fmt.Sprintf("%s.images[%d]", bat, j)
		switch {
		case strings.TrimSpace(img) == "":
			v.addf("%s: required", iat)
		case !imageRe.MatchString(img):
			v.addf("%s: %q is not an image reference", iat, img)
		}
		for _, m := range placeholderRe.FindAllStringSubmatch(img, -1) {
			if !slices.Contains(BuildArgPlaceholders, m[1]) {
				v.addf("%s: unknown placeholder ${%s} (want %s)", iat, m[1], list(placeholders(BuildArgPlaceholders)))
			}
		}
	}
	for j, f := range b.VersionFiles {
		fat := fmt.Sprintf("%s.version_files[%d]", bat, j)
		if problem := v.pathProblem(fat, f); problem {
			continue
		}
		if !slices.Contains(VersionFileExtensions, strings.ToLower(path.Ext(normalize(f)))) {
			v.addf("%s: %q is not an MSBuild project file (%s)", fat, f, list(VersionFileExtensions))
		}
	}
	if c := b.Coverage; c != nil {
		cat := bat + ".coverage"
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

// pathProblem is checkPath reporting whether it found a problem.
func (v *validator) pathProblem(at, raw string) bool {
	n := len(v.problems)
	v.checkPath(at, raw)
	return len(v.problems) > n
}

// removedKey is a project key nerv ship no longer reads, with what to
// write instead.
type removedKey struct {
	key, message string
}

// removedProjectKeys are the preset keys removed with the presets: Load
// reports each with its replacement instead of an unknown field.
var removedProjectKeys = []removedKey{
	{"presets", `the presets were removed; set build.mode instead: "build: { mode: script }" with each stage's command under stages (e.g. ` +
		`"stages: { build: { run: "dotnet build -c Release" } }"), or ` +
		`"build: { mode: compose, file: docker-compose.yml, test_service: tests, images: ["ghcr.io/org/api:${version}"] }"`},
	{"dotnet", `the dotnet preset was removed; with build: { mode: script } write its commands as stages, e.g. ` +
		`"stages: { restore: { run: "dotnet restore" }, lint: { run: "dotnet format --verify-no-changes" }, build: { run: "dotnet build -c Release --no-restore" }, test: { run: "dotnet test -c Release --no-build" } }"; ` +
		`project_file goes to build.version_files, coverage_threshold to build.coverage (line, branch, method; the tests write *.cobertura.xml under build.coverage.dir), ` +
		`shared_host to build.shared_host, and for sdk: write global.json in a before_restore hook`},
	{"node", `the node preset was removed; with build: { mode: script } write its commands as stages, e.g. ` +
		`"stages: { restore: { run: "npm ci" }, lint: { run: "npm run lint" }, build: { run: "npm run build" }, test: { run: "npm test" } }"`},
	{"docker", `the docker preset was removed; use ` +
		`"build: { mode: compose, file: docker-compose.yml, images: ["ghcr.io/org/api:${version}"] }", or with build: { mode: script } a package stage such as ` +
		`"stages: { package: { run: "docker build -t ghcr.io/org/api:$NERV_RELEASE_VERSION . && docker save -o \"$NERV_RELEASE_OUTPUTS_DIR/api.tar\" ghcr.io/org/api:$NERV_RELEASE_VERSION" } }"; ` +
		`pushing moved to nerv deploy`},
	{"nuget", `the nuget preset was removed; package in a script stage instead: ` +
		`"stages: { package: { run: "dotnet pack -c Release --no-build -o \"$NERV_RELEASE_OUTPUTS_DIR\"" } }"; pushing moved to nerv deploy`},
	{"npm", `the npm preset was removed; package in a script stage instead: ` +
		`"stages: { package: { run: "npm pack --pack-destination \"$NERV_RELEASE_OUTPUTS_DIR\"" } }"; publishing moved to nerv deploy`},
}

// removedStages and removedHooks are the stage and hook keys removed from
// nerv ship, with what to do instead.
var (
	removedStages = map[string]string{
		"publish": "the publish stage was removed from nerv ship (publishing moved to nerv deploy); write the outputs to $NERV_RELEASE_OUTPUTS_DIR in the package stage instead",
	}
	removedHooks = map[string]string{
		"publish": "the publish hooks were removed from nerv ship: they belong to nerv deploy now",
		"success": "the success hooks were removed from nerv ship: they belong to nerv deploy now",
		// There is no publish stage to hook before or after.
		"before_publish": "the publish stage moved to nerv deploy (the deploy: section's targets), so there is nothing to hook: run it from a deploy target, or use the prepare hook to act before the artifact is written",
		"after_publish":  "the publish stage moved to nerv deploy (the deploy: section's targets), so there is nothing to hook: run it from a deploy target after the push",
	}
)

// takeRemovedKeys reports every removed preset key of the projects of the
// ship: node and deletes it from the node, so it is neither decoded nor
// reported as an unknown field.
func takeRemovedKeys(release *yaml.Node) []string {
	problems := takeRemovedSectionKeys(release)
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
			problems = append(problems, fmt.Sprintf("ship.projects[%d].%s: %s", i, key, removedProjectKeys[idx].message))
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
