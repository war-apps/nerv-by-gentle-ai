package migrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig/yamlsection"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseplan"
	"gopkg.in/yaml.v3"
)

// buildProvider is the build provider a 4.2.0 project gets: the presets ran
// commands, which the bash provider's stages run.
const buildProvider = "bash"

// TODOPrefix starts every comment nerv migrate writes where a 4.2.0
// behaviour has no exact equivalent.
const TODOPrefix = "TODO(nerv migrate):"

// The 4.2.0 defaults the presets used (internal/releaseconfig at 4.2.0).
const (
	oldDotnetConfiguration = "Release"
	oldPackageManager      = "npm"
	oldDockerfile          = "Dockerfile"
	oldDockerContext       = "."
	oldNuGetSource         = "https://api.nuget.org/v3/index.json"
	oldNuGetAPIKeyEnv      = "NUGET_API_KEY"
	oldNpmAccess           = "public"
)

// oldDockerTags is the 4.2.0 default image tag kinds.
var oldDockerTags = []string{"version", "channel", "latest"}

// oldPresetKeys are the 4.2.0 project keys of the presets.
var oldPresetKeys = []string{"presets", "dotnet", "node", "docker", "nuget", "npm"}

// oldStages are the 4.2.0 stages, in run order (publish moved to deploy).
var oldStages = []string{"restore", "lint", "build", "test", "coverage", "package", "publish"}

// removedHooks are the 4.2.0 hook keys nerv release no longer has.
var removedHooks = map[string]string{
	"publish":        "it ran at semantic-release's publish step, which is nerv deploy's now",
	"success":        "it ran after semantic-release published, which is nerv deploy's now",
	"before_publish": "the publish stage moved to nerv deploy",
	"after_publish":  "the publish stage moved to nerv deploy",
}

// oldProject is what the translation reads of a 4.2.0 release project.
type oldProject struct {
	Name    string   `yaml:"name"`
	Path    string   `yaml:"path"`
	Presets []string `yaml:"presets"`
	Dotnet  *struct {
		ProjectFile       string  `yaml:"project_file"`
		Solution          string  `yaml:"solution"`
		CoverageThreshold float64 `yaml:"coverage_threshold"`
		Configuration     string  `yaml:"configuration"`
	} `yaml:"dotnet"`
	Node *struct {
		PackageManager string `yaml:"package_manager"`
	} `yaml:"node"`
	Docker *struct {
		Images []oldImage `yaml:"images"`
	} `yaml:"docker"`
	NuGet *struct {
		Source    string `yaml:"source"`
		APIKeyEnv string `yaml:"api_key_env"`
	} `yaml:"nuget"`
	Npm *struct {
		Registry string `yaml:"registry"`
		Access   string `yaml:"access"`
		DistTag  string `yaml:"dist_tag"`
	} `yaml:"npm"`
	Stages map[string]yaml.Node `yaml:"stages"`
}

type oldImage struct {
	Name       string            `yaml:"name"`
	Dockerfile string            `yaml:"dockerfile"`
	Context    string            `yaml:"context"`
	BuildArgs  map[string]string `yaml:"build_args"`
	Tags       []string          `yaml:"tags"`
	Push       *bool             `yaml:"push"`
}

// step is one generated stage command.
type step struct {
	run     string
	env     map[string]string
	workdir string
}

// stagePlan is the generated commands of one project, per stage, and the
// TODO comments of each stage.
type stagePlan struct {
	steps map[string][]step
	todos map[string][]string
}

func (s *stagePlan) add(stage string, st step) { s.steps[stage] = append(s.steps[stage], st) }
func (s *stagePlan) todo(stage, format string, args ...any) {
	s.todos[stage] = append(s.todos[stage], TODOPrefix+" "+fmt.Sprintf(format, args...))
}

// releaseTranslation is the release: section translated from the 4.2.0 one.
type releaseTranslation struct {
	// block is the "release:" block text.
	block string
}

// translateRelease turns the 4.2.0 release: section of data into today's
// release: section: semantic_release is dropped, each project's presets
// become build: bash with the commands the 4.2.0 presets ran (its
// own stage overrides still win, as in 4.2.0), the publish stage and
// hooks go (deploy publishes), and every other key and comment is kept.
func translateRelease(p *Plan, data []byte, repoRoot string) (*releaseTranslation, error) {
	node, err := yamlsection.Locate(data, "release")
	if err != nil {
		return nil, refuse("release: cannot be read: %v", err)
	}
	if node.Kind != yaml.MappingNode {
		return nil, refuse("release: must be a mapping")
	}
	var content []*yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		k, v := node.Content[i], node.Content[i+1]
		switch k.Value {
		case "semantic_release":
			p.note("release.semantic_release dropped: the version engine is native (no semantic-release, node or npm)")
			continue
		case "projects":
			if v.Kind == yaml.SequenceNode {
				for j, proj := range v.Content {
					if err := translateProject(p, proj, j, repoRoot); err != nil {
						return nil, err
					}
				}
			}
		}
		content = append(content, k, v)
	}
	node.Content = content
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: releaseconfig.SectionKey}, node}}
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	p.note("release: converted from the 4.2.0 section")
	return &releaseTranslation{block: strings.TrimRight(b.String(), "\n")}, nil
}

// translateProject rewrites one release project node in place.
func translateProject(p *Plan, node *yaml.Node, index int, repoRoot string) error {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	var old oldProject
	if err := node.Decode(&old); err != nil {
		return refuse("release.projects[%d]: cannot be read: %v", index, err)
	}
	at := fmt.Sprintf("release.projects[%d] (%s)", index, old.Name)
	old.Path = yamlsection.NormalizePath(old.Path)
	plan := &stagePlan{steps: map[string][]step{}, todos: map[string][]string{}}
	// build collects the project keys the presets give (coverage,
	// version_files, images), written right after build: bash.
	build := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, preset := range old.Presets {
		switch preset {
		case "dotnet":
			dotnetPreset(p, plan, build, old, at)
		case "node":
			nodePreset(p, plan, old, repoRoot, at)
		case "docker":
			dockerPreset(p, plan, build, old, at)
		case "nuget":
			nugetPreset(p, plan, old, at)
		case "npm":
			npmPreset(p, plan, old, at)
		default:
			p.warn("%s: unknown 4.2.0 preset %q dropped", at, preset)
		}
	}
	if len(old.Presets) > 0 {
		p.note("%s: presets %s became build: bash with the commands they ran", at, strings.Join(old.Presets, ", "))
	}

	var content []*yaml.Node
	buildPlaced := false
	placeBuild := func() {
		if !buildPlaced {
			content = append(content, scalar("build"), scalar(buildProvider))
			content = append(content, build.Content...)
			buildPlaced = true
		}
	}
	var stagesNode *yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		k, v := node.Content[i], node.Content[i+1]
		switch {
		case slices.Contains(oldPresetKeys, k.Value):
			placeBuild()
			continue
		case k.Value == "stages":
			placeBuild()
			stagesNode = v
		case k.Value == "hooks":
			placeBuild()
			filterHooks(p, v, at)
			if len(v.Content) == 0 {
				continue
			}
		}
		content = append(content, k, v)
	}
	placeBuild()
	if stagesNode == nil && len(plan.steps) > 0 {
		stagesNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		// The stages go right after build: and the keys the presets gave.
		idx := slices.IndexFunc(content, func(n *yaml.Node) bool { return n.Value == "build" })
		content = slices.Insert(content, idx+2+len(build.Content), scalar("stages"), stagesNode)
	}
	if stagesNode != nil {
		mergeStages(p, stagesNode, plan, at)
		if len(stagesNode.Content) == 0 {
			idx := slices.IndexFunc(content, func(n *yaml.Node) bool { return n.Value == "stages" })
			content = slices.Delete(content, idx, idx+2)
		}
	}
	if stagesNode == nil || len(stagesNode.Content) == 0 {
		p.warn("%s: no stage commands: nerv release would package nothing; write the project's stages (build: bash)", at)
	}
	node.Content = content
	return nil
}

// mergeStages orders the stages node as the stages run: each stage's own
// override (kept with its comments), else the preset commands; a publish
// stage is dropped.
func mergeStages(p *Plan, stages *yaml.Node, plan *stagePlan, at string) {
	own := map[string][2]*yaml.Node{}
	var order []string
	for i := 0; i+1 < len(stages.Content); i += 2 {
		k := stages.Content[i]
		own[k.Value] = [2]*yaml.Node{k, stages.Content[i+1]}
		order = append(order, k.Value)
	}
	var content []*yaml.Node
	for _, stage := range oldStages {
		if kv, ok := own[stage]; ok {
			if stage == "publish" {
				p.warn("%s: stages.publish dropped: publishing moved to nerv deploy (a deploy target); its command was: %s", at, strings.TrimSpace(scalarValue(value(kv[1], "run"))))
				continue
			}
			if len(plan.steps[stage]) > 0 {
				p.note("%s: stages.%s keeps its own command, which replaced the presets' in 4.2.0 too", at, stage)
			}
			content = append(content, kv[0], kv[1])
			continue
		}
		steps := plan.steps[stage]
		if len(steps) == 0 {
			continue
		}
		if stage == "publish" {
			continue
		}
		k := scalar(stage)
		if todos := plan.todos[stage]; len(todos) > 0 {
			k.HeadComment = strings.Join(todos, "\n")
		}
		content = append(content, k, commandNode(combine(steps)))
	}
	for _, name := range order {
		if !slices.Contains(oldStages, name) {
			content = append(content, own[name][0], own[name][1])
		}
	}
	stages.Content = content
}

// filterHooks drops the hooks of the removed lifecycle keys.
func filterHooks(p *Plan, hooks *yaml.Node, at string) {
	if hooks.Kind != yaml.MappingNode {
		return
	}
	var kept []*yaml.Node
	for i := 0; i+1 < len(hooks.Content); i += 2 {
		k := hooks.Content[i]
		if why, removed := removedHooks[k.Value]; removed {
			var runs []string
			for _, c := range hooks.Content[i+1].Content {
				runs = append(runs, strings.TrimSpace(scalarValue(value(c, "run"))))
			}
			p.warn("%s: hooks.%s dropped (%s); its commands were: %s", at, k.Value, why, strings.Join(runs, "; "))
			continue
		}
		kept = append(kept, k, hooks.Content[i+1])
	}
	hooks.Content = kept
}

// combine joins the commands several presets wrote for one stage into one
// script, each in its own directory and environment as 4.2.0 ran them.
func combine(steps []step) step {
	if len(steps) == 1 {
		return steps[0]
	}
	var lines []string
	for _, s := range steps {
		line := s.run
		if len(s.env) > 0 {
			var assign []string
			for _, k := range slices.Sorted(maps.Keys(s.env)) {
				assign = append(assign, k+"="+shq(s.env[k]))
			}
			line = strings.Join(assign, " ") + " " + line
		}
		if s.workdir != "" {
			line = "(cd " + shq(s.workdir) + " && " + line + ")"
		}
		lines = append(lines, line)
	}
	return step{run: strings.Join(lines, "\n") + "\n"}
}

// commandNode is a stage command mapping.
func commandNode(s step) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	run := scalar(s.run)
	if strings.Contains(s.run, "\n") {
		run.Style = yaml.LiteralStyle
	}
	addKV(m, "run", run)
	if len(s.env) > 0 {
		env := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
		for _, k := range slices.Sorted(maps.Keys(s.env)) {
			v := scalar(s.env[k])
			v.Style = yaml.DoubleQuotedStyle
			addKV(env, k, v)
		}
		addKV(m, "env", env)
	}
	if s.workdir != "" {
		addKV(m, "workdir", scalar(s.workdir))
	}
	return m
}

func scalar(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }

func addKV(m *yaml.Node, k string, v *yaml.Node) { m.Content = append(m.Content, scalar(k), v) }

func scalarValue(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}

// safeWord is a shell word that needs no quoting.
var safeWord = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// shq quotes s for bash when it needs it.
func shq(s string) string {
	if safeWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

const (
	versionVar = `"$NERV_RELEASE_VERSION"`
	tagVar     = `"$NERV_RELEASE_TAG"`
)

// dotnetPreset is 4.2.0's dotnet preset: restore, format check, build with
// the version, test (collecting coverage when a threshold is set) and the
// coverage check, every command without build servers; the csproj version
// write is the project's version_files.
func dotnetPreset(p *Plan, plan *stagePlan, build *yaml.Node, old oldProject, at string) {
	d := old.Dotnet
	if d == nil || d.ProjectFile == "" {
		p.warn("%s: the dotnet preset had no dotnet.project_file (4.2.0 refused it): no dotnet commands written", at)
		return
	}
	cfg := d.Configuration
	if cfg == "" {
		cfg = oldDotnetConfiguration
	}
	target := yamlsection.NormalizePath(d.Solution)
	if target == "" {
		target = yamlsection.NormalizePath(d.ProjectFile)
	}
	t := shq(target)
	env := releaseplan.DotnetBuildServerEnv
	noServers := releaseplan.DisableBuildServers
	plan.add("restore", step{run: "dotnet restore " + t + " " + noServers, env: env()})
	plan.add("lint", step{run: "dotnet format " + t + " --verify-no-changes", env: env()})
	plan.add("build", step{run: "dotnet build " + t + " -c " + shq(cfg) + " --no-restore " + noServers + " -p:Version=" + versionVar, env: env()})
	test := "dotnet test " + t + " -c " + shq(cfg) + " --no-build " + noServers
	if d.CoverageThreshold > 0 {
		if _, own := old.Stages["coverage"]; own {
			p.warn("%s: dotnet.coverage_threshold %s is not checked: the project's own coverage stage replaced the check in 4.2.0 too", at, formatFloat(d.CoverageThreshold))
		} else {
			test += ` --collect "XPlat Code Coverage" --results-directory "$NERV_RELEASE_ARTIFACTS_DIR/coverage"`
			cov := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			tag := "!!float"
			if d.CoverageThreshold == float64(int64(d.CoverageThreshold)) {
				tag = "!!int"
			}
			addKV(cov, "line", &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: formatFloat(d.CoverageThreshold)})
			addKV(build, "coverage", cov)
		}
	}
	plan.add("test", step{run: test, env: env()})
	plan.todo("restore", "4.2.0 dropped %s from these dotnet commands when the .NET SDK was older than %s; remove it if yours is", noServers, releaseplan.MinBuildServersSDK)
	p.warn("%s: the dotnet commands keep %s: 4.2.0 dropped it for a .NET SDK older than %s, script mode does not (see the TODO in release.projects stages.restore)", at, noServers, releaseplan.MinBuildServersSDK)
	files := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle, Content: []*yaml.Node{scalar(yamlsection.NormalizePath(d.ProjectFile))}}
	addKV(build, "version_files", files)
	if slices.Contains(old.Presets, "nuget") {
		plan.add("package", step{
			run: "dotnet pack " + shq(yamlsection.NormalizePath(d.ProjectFile)) + " -c " + shq(cfg) + " --no-build " + noServers +
				" -p:Version=" + versionVar + ` -o "$NERV_RELEASE_OUTPUTS_DIR"`,
			env: env(),
		})
	}
}

func formatFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// nodePreset is 4.2.0's node preset: a frozen-lockfile install, then the
// package's lint, build and test scripts that package.json defines.
func nodePreset(p *Plan, plan *stagePlan, old oldProject, repoRoot, at string) {
	pm := oldPackageManager
	if old.Node != nil && old.Node.PackageManager != "" {
		pm = old.Node.PackageManager
	}
	workdir := ""
	if old.Path != "." && old.Path != "" {
		workdir = old.Path
	}
	dir := filepath.Join(repoRoot, filepath.FromSlash(old.Path))
	var pkg struct {
		PackageManager string            `json:"packageManager"`
		Scripts        map[string]string `json:"scripts"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err == nil {
		err = json.Unmarshal(data, &pkg)
	}
	if err != nil {
		p.warn("%s: the node preset reads %s: %v; only the install is written", at, path.Join(old.Path, "package.json"), err)
	}
	var install string
	switch pm {
	case "pnpm":
		install = "pnpm install --frozen-lockfile"
	case "yarn":
		if yarnBerry(pkg.PackageManager, dir, repoRoot) {
			install = "yarn install --immutable"
		} else {
			install = "yarn install --frozen-lockfile"
		}
	default:
		install = "npm ci"
	}
	plan.add("restore", step{run: install, workdir: workdir})
	for _, script := range []string{"lint", "build", "test"} {
		if _, ok := pkg.Scripts[script]; ok {
			plan.add(script, step{run: pm + " run " + script, workdir: workdir})
		}
	}
}

// yarnBerry is 4.2.0's Yarn 2+ detection.
func yarnBerry(packageManager, dir, root string) bool {
	if v, ok := strings.CutPrefix(packageManager, "yarn@"); ok {
		major, _, _ := strings.Cut(v, ".")
		n, err := strconv.Atoi(major)
		return err == nil && n >= 2
	}
	for _, d := range []string{dir, root} {
		if _, err := os.Stat(filepath.Join(d, ".yarnrc.yml")); err == nil {
			return true
		}
	}
	return false
}

var placeholderRe = regexp.MustCompile(`\$\{([a-z_]+)\}`)

// dockerTagsPreamble sets the channel and latest tags as 4.2.0 chose them:
// the channel tag off the default channel, latest only on it.
const dockerTagsPreamble = `channel_tag=""
latest_tag=""
if [ -z "$NERV_RELEASE_CHANNEL" ] || [ "$NERV_RELEASE_CHANNEL" = default ]; then latest_tag=latest; else channel_tag="$NERV_RELEASE_CHANNEL"; fi`

// dockerPreset is 4.2.0's docker preset, package side: one docker build per
// image with its tags, build arguments and labels. Each image 4.2.0 pushed
// becomes an images mapping {name, tags} with its 4.2.0 tag kinds:
// nerv release tags it, saves every tag into the artifact after the package
// commands and records each reference, and deploy-ssh-compose's publish
// pushes them all. An image 4.2.0 did not push
// (push: false) is saved by the package commands themselves, so the
// artifact holds it without a reference to push.
func dockerPreset(p *Plan, plan *stagePlan, build *yaml.Node, old oldProject, at string) {
	if old.Docker == nil || len(old.Docker.Images) == 0 {
		p.warn("%s: the docker preset had no docker.images (4.2.0 refused it): no docker commands written", at)
		return
	}
	lines := []string{dockerTagsPreamble}
	images := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	tars := map[string]bool{}
	for _, img := range old.Docker.Images {
		pushed := img.Push == nil || *img.Push
		kinds := img.Tags
		if len(kinds) == 0 {
			kinds = oldDockerTags
		}
		var known []string // the 4.2.0 tag kinds, deduplicated, for images
		for _, kind := range kinds {
			if slices.Contains(oldDockerTags, kind) && !slices.Contains(known, kind) {
				known = append(known, kind)
			}
		}
		kinds = slices.Compact(slices.Clone(kinds))
		if pushed && !slices.Contains(kinds, "version") {
			// nerv release tags the others from <name>:<version>, so it must be built.
			kinds = append([]string{"version"}, kinds...)
		}
		var tags []string // shell words, without -t
		for _, kind := range kinds {
			switch kind {
			case "version":
				tags = append(tags, img.Name+":"+versionVar)
			case "channel":
				tags = append(tags, "${channel_tag:+"+img.Name+":$channel_tag}")
			case "latest":
				tags = append(tags, "${latest_tag:+"+img.Name+":latest}")
			default:
				p.warn("%s: docker image %s: unknown tag kind %q dropped", at, img.Name, kind)
			}
		}
		dockerfile, context := yamlsection.NormalizePath(img.Dockerfile), yamlsection.NormalizePath(img.Context)
		if dockerfile == "" {
			dockerfile = oldDockerfile
		}
		if context == "" {
			context = oldDockerContext
		}
		cmd := []string{"docker build -f " + shq(dockerfile)}
		for _, t := range tags {
			if strings.HasPrefix(t, "${") {
				// ${x:+name:$x} expands to one word or none.
				cmd = append(cmd, strings.Replace(t, ":+", ":+-t ", 1))
				continue
			}
			cmd = append(cmd, "-t "+t)
		}
		for _, k := range slices.Sorted(maps.Keys(img.BuildArgs)) {
			cmd = append(cmd, "--build-arg "+expandWord(k+"="+img.BuildArgs[k]))
		}
		cmd = append(cmd, "--label org.opencontainers.image.version="+versionVar,
			"${NERV_RELEASE_GIT_HEAD:+--label org.opencontainers.image.revision=$NERV_RELEASE_GIT_HEAD}", shq(context))
		lines = append(lines, strings.Join(cmd, " "))
		if pushed {
			tagList := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
			for _, kind := range known {
				tagList.Content = append(tagList.Content, scalar(kind))
			}
			entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
			addKV(entry, "name", scalar(img.Name))
			addKV(entry, "tags", tagList)
			images.Content = append(images.Content, entry)
			continue
		}
		tar := path.Base(img.Name)
		for i := 2; tars[tar]; i++ {
			tar = path.Base(img.Name) + "-" + strconv.Itoa(i)
		}
		tars[tar] = true
		lines = append(lines, `docker save -o "$NERV_RELEASE_OUTPUTS_DIR/`+tar+`.tar" `+strings.Join(tags, " "))
	}
	plan.add("package", step{run: strings.Join(lines, "\n") + "\n"})
	if len(images.Content) > 0 {
		addKV(build, "images", images)
		p.note("%s: the docker images 4.2.0 pushed became the project's images with their 4.2.0 tags: nerv release tags and saves them into the artifact and deploy-ssh-compose pushes every tag", at)
	}
}

// expandWord is a shell word for a 4.2.0 build argument, its ${version},
// ${channel} and ${tag} placeholders read from the release variables.
func expandWord(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range placeholderRe.FindAllStringSubmatchIndex(s, -1) {
		if lit := s[last:m[0]]; lit != "" {
			b.WriteString(shq(lit))
		}
		switch s[m[2]:m[3]] {
		case "version":
			b.WriteString(versionVar)
		case "channel":
			b.WriteString(`"$channel_tag"`)
		case "tag":
			b.WriteString(tagVar)
		default:
			b.WriteString(shq(s[m[0]:m[1]]))
		}
		last = m[1]
	}
	if lit := s[last:]; lit != "" || b.Len() == 0 {
		b.WriteString(shq(lit))
	}
	return b.String()
}

// nugetPreset is 4.2.0's nuget preset: dotnet pack (written with the
// dotnet preset) and a push, which is nerv deploy's now.
func nugetPreset(p *Plan, plan *stagePlan, old oldProject, at string) {
	source, keyEnv := oldNuGetSource, oldNuGetAPIKeyEnv
	if n := old.NuGet; n != nil {
		if n.Source != "" {
			source = n.Source
		}
		if n.APIKeyEnv != "" {
			keyEnv = n.APIKeyEnv
		}
	}
	if !slices.Contains(old.Presets, "dotnet") {
		p.warn("%s: the nuget preset packed through the dotnet preset or a package stage; no dotnet pack written", at)
	}
	plan.todo("package", "4.2.0 pushed the packages to %s with the API key from $%s; no deploy provider pushes NuGet packages yet: push them from a deploy target or by hand", source, keyEnv)
	p.warn("%s: nuget push to %s ($%s) has no deploy provider yet (see the TODO in stages.package)", at, source, keyEnv)
}

// npmPreset is 4.2.0's npm preset: npm publish, which is nerv deploy's now;
// release packs the package into the artifact.
func npmPreset(p *Plan, plan *stagePlan, old oldProject, at string) {
	access := oldNpmAccess
	publish := "npm publish --access "
	if n := old.Npm; n != nil && n.Access != "" {
		access = n.Access
	}
	publish += access
	if n := old.Npm; n != nil {
		if n.DistTag != "" {
			publish += " --tag " + shq(n.DistTag)
		}
		if n.Registry != "" {
			publish += " --registry " + shq(n.Registry)
		}
	}
	workdir := ""
	if old.Path != "." && old.Path != "" {
		workdir = old.Path
	}
	plan.add("package", step{run: `npm pack --pack-destination "$NERV_RELEASE_OUTPUTS_DIR"`, workdir: workdir})
	plan.todo("package", "4.2.0 ran %q (with the release channel as the dist-tag off the default channel); no deploy provider publishes to npm yet: publish the packed tarball from a deploy target or by hand", publish)
	p.warn("%s: npm publish has no deploy provider yet (see the TODO in stages.package)", at)
}
