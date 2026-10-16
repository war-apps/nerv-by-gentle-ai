package shipplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// dotnet plans the dotnet preset, as dim.comprobantes.api's CI runs it:
// restore, format check, build (with the release version), test, and with
// a coverage threshold the tests collect coverage (one test run, as in CI)
// and the coverage stage checks it; dotnet pack only for the nuget preset.
func (b *builder) dotnet() error {
	d := b.project.Dotnet
	if d == nil || d.ProjectFile == "" {
		return errors.New("the dotnet preset needs dotnet.project_file")
	}
	cfg := d.Configuration
	if cfg == "" {
		cfg = shipconfig.DefaultDotnetConfiguration
	}
	target := d.Solution
	if target == "" {
		target = d.ProjectFile
	}
	version := "-p:Version=" + b.ctx.Version
	// Every dotnet command runs without build servers (DotnetBuildServerEnv);
	// format takes the environment but not the flag.
	noServers := func() shipconfig.Command { return shipconfig.Command{Env: DotnetBuildServerEnv()} }
	b.add("restore", Step{Name: "dotnet:restore", Command: noServers(), Exec: []string{"dotnet", "restore", target, DisableBuildServers}})
	b.add("lint", Step{Name: "dotnet:lint", Command: noServers(), Exec: []string{"dotnet", "format", target, "--verify-no-changes"}})
	b.add("build", Step{Name: "dotnet:build", Command: noServers(), Exec: []string{"dotnet", "build", target, "-c", cfg, "--no-restore", DisableBuildServers, version}})
	test := []string{"dotnet", "test", target, "-c", cfg, "--no-build", DisableBuildServers}
	if threshold := d.CoverageThreshold; threshold > 0 {
		dir := filepath.Join(b.artifacts, "coverage")
		b.resetDirs = append(b.resetDirs, dir)
		test = append(test, "--collect", "XPlat Code Coverage", "--results-directory", dir)
		b.add("coverage", Step{
			Name:     "dotnet:coverage",
			Describe: fmt.Sprintf("check line coverage >= %s%% in %s", strconv.FormatFloat(threshold, 'f', -1, 64), dir),
			Func: func(context.Context, func(string) (string, bool)) (string, error) {
				c, err := CheckCoverage(dir, threshold)
				if err != nil {
					return "", err
				}
				return c.String(), nil
			},
		})
	}
	b.add("test", Step{Name: "dotnet:test", Command: noServers(), Exec: test})
	if b.enabled("nuget") {
		b.add("package", Step{Name: "dotnet:pack", Command: noServers(), Exec: []string{"dotnet", "pack", d.ProjectFile, "-c", cfg, "--no-build", DisableBuildServers, version, "-o", b.packagesDir()}})
	}
	return nil
}

// packagesDir is where dotnet pack writes and nuget push reads.
func (b *builder) packagesDir() string { return filepath.Join(b.artifacts, "nuget") }

// workdir is the project's path as a command workdir ("" for the root).
func (b *builder) workdir() string {
	if b.project.Path == "." {
		return ""
	}
	return b.project.Path
}

// packageJSON is what the node preset reads from the project's package.json.
type packageJSON struct {
	PackageManager string            `json:"packageManager"`
	Scripts        map[string]string `json:"scripts"`
}

// node plans the node preset: a frozen-lockfile install, then the
// package's lint, build and test scripts, each only when package.json
// defines it. Versions are left to semantic-release.
func (b *builder) node() error {
	pm := shipconfig.DefaultPackageManager
	if b.project.Node != nil && b.project.Node.PackageManager != "" {
		pm = b.project.Node.PackageManager
	}
	dir := filepath.Join(b.root, filepath.FromSlash(b.project.Path))
	rel := filepath.Join(filepath.FromSlash(b.project.Path), "package.json")
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return fmt.Errorf("the node preset reads %s: %w", rel, err)
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("the node preset reads %s: %w", rel, err)
	}
	cmd := shipconfig.Command{Workdir: b.workdir()}
	var install []string
	switch pm {
	case "pnpm":
		install = []string{"pnpm", "install", "--frozen-lockfile"}
	case "yarn":
		if yarnBerry(pkg.PackageManager, dir, b.root) {
			install = []string{"yarn", "install", "--immutable"}
		} else {
			install = []string{"yarn", "install", "--frozen-lockfile"}
		}
	default:
		install = []string{"npm", "ci"}
	}
	b.add("restore", Step{Name: "node:restore", Command: cmd, Exec: install})
	for _, script := range []string{"lint", "build", "test"} {
		if _, ok := pkg.Scripts[script]; ok {
			b.add(script, Step{Name: "node:" + script, Command: cmd, Exec: []string{pm, "run", script}})
		}
	}
	return nil
}

// yarnBerry reports whether the project uses Yarn 2 or newer (whose
// install takes --immutable instead of --frozen-lockfile): package.json's
// packageManager names yarn@2+, or a .yarnrc.yml (berry's config file) is
// in the project directory or the repository root.
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

// dockerTagRe is a valid docker image tag.
var dockerTagRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// placeholderRe matches a ${name} build-arg placeholder.
var placeholderRe = regexp.MustCompile(`\$\{([a-z_]+)\}`)

// docker plans the docker preset: one docker build per image in the
// package stage, and one docker push per tag in the publish stage.
func (b *builder) docker() error {
	if b.project.Docker == nil {
		return errors.New("the docker preset needs docker.images")
	}
	for _, img := range b.project.Docker.Images {
		tags, err := b.dockerTags(img)
		if err != nil {
			return fmt.Errorf("docker image %s: %w", img.Name, err)
		}
		dockerfile, buildContext := img.Dockerfile, img.Context
		if dockerfile == "" {
			dockerfile = shipconfig.DefaultDockerfile
		}
		if buildContext == "" {
			buildContext = shipconfig.DefaultDockerContext
		}
		build := []string{"docker", "build", "-f", dockerfile}
		for _, t := range tags {
			build = append(build, "-t", img.Name+":"+t)
		}
		for _, k := range slices.Sorted(maps.Keys(img.BuildArgs)) {
			build = append(build, "--build-arg", k+"="+b.expand(img.BuildArgs[k]))
		}
		build = append(build, "--label", "org.opencontainers.image.version="+b.ctx.Version)
		if b.ctx.GitHead != "" {
			build = append(build, "--label", "org.opencontainers.image.revision="+b.ctx.GitHead)
		}
		build = append(build, buildContext)
		b.add("package", Step{Name: "docker:build " + img.Name, Exec: build})
		if img.ShouldPush() {
			for _, t := range tags {
				ref := img.Name + ":" + t
				b.add(PublishStage, Step{Name: "docker:push " + ref, Exec: []string{"docker", "push", ref}})
			}
		}
	}
	return nil
}

// dockerTags resolves an image's tag kinds for this release, as
// dim.comprobantes.api's workflow does: version is always applied; channel
// only when the channel is not the default one; latest only on the
// default channel. Duplicates are dropped and every tag must be valid.
func (b *builder) dockerTags(img shipconfig.DockerImage) ([]string, error) {
	kinds := img.Tags
	if len(kinds) == 0 {
		kinds = shipconfig.DefaultDockerTags()
	}
	var tags []string
	for _, kind := range kinds {
		var t string
		switch kind {
		case "version":
			t = b.ctx.Version
		case "channel":
			if !b.ctx.defaultChannel() {
				t = b.ctx.Channel
			}
		case "latest":
			if b.ctx.defaultChannel() {
				t = "latest"
			}
		default:
			return nil, fmt.Errorf("unknown tag kind %q", kind)
		}
		if t == "" || slices.Contains(tags, t) {
			continue
		}
		if !dockerTagRe.MatchString(t) {
			return nil, fmt.Errorf("%s tag %q is not a valid docker tag ([A-Za-z0-9_][A-Za-z0-9_.-]*, at most 128 characters)", kind, t)
		}
		tags = append(tags, t)
	}
	return tags, nil
}

// expand replaces the build-arg placeholders ${version}, ${channel} and
// ${tag} (the channel is empty on the default channel).
func (b *builder) expand(s string) string {
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		switch placeholderRe.FindStringSubmatch(m)[1] {
		case "version":
			return b.ctx.Version
		case "channel":
			if b.ctx.defaultChannel() {
				return ""
			}
			return b.ctx.Channel
		case "tag":
			return b.ctx.Tag
		}
		return m
	})
}

// nuget plans the nuget preset: an in-process push (pushNuGet) of the
// packages dotnet pack wrote, with the API key read from api_key_env when
// the step runs and sent only in the push requests' header.
func (b *builder) nuget() error {
	n := b.project.NuGet
	if n == nil {
		n = &shipconfig.NuGet{}
	}
	source, keyEnv := n.Source, n.APIKeyEnv
	if source == "" {
		source = shipconfig.DefaultNuGetSource
	}
	if keyEnv == "" {
		keyEnv = shipconfig.DefaultNuGetAPIKeyEnv
	}
	if _, overridden := b.project.Stages["package"]; !b.enabled("dotnet") && !overridden {
		return errors.New("the nuget preset needs the dotnet preset (its pack writes the packages) or a package stage override writing *.nupkg to $" + ArtifactsEnv + "/nuget")
	}
	if err := checkPushURL(source); err != nil {
		return fmt.Errorf("nuget.source: %w", err)
	}
	dir := b.packagesDir()
	b.resetDirs = append(b.resetDirs, dir)
	b.add(PublishStage, Step{
		Name:     "nuget:push",
		Describe: fmt.Sprintf("push %s (and *.snupkg) to %s with the API key from $%s, skipping packages already there", filepath.Join(dir, "*.nupkg"), source, keyEnv),
		Secrets:  []string{keyEnv},
		Func: func(ctx context.Context, lookupEnv func(string) (string, bool)) (string, error) {
			key, _ := lookupEnv(keyEnv)
			return pushNuGet(ctx, nil, dir, source, key)
		},
	})
	return nil
}

// npm plans the npm preset: npm publish from the project directory, with
// the dist-tag dist_tag, else the release channel (none on the default
// channel, so npm uses latest).
func (b *builder) npm() error {
	n := b.project.Npm
	if n == nil {
		n = &shipconfig.Npm{}
	}
	access := n.Access
	if access == "" {
		access = shipconfig.DefaultNpmAccess
	}
	publish := []string{"npm", "publish", "--access", access}
	tag, tagFrom := n.DistTag, "npm.dist_tag"
	if tag == "" && !b.ctx.defaultChannel() {
		tag, tagFrom = b.ctx.Channel, "the release channel"
	}
	if tag != "" {
		if err := checkCmdSafe(tagFrom, tag); err != nil {
			return err
		}
		publish = append(publish, "--tag", tag)
	}
	if n.Registry != "" {
		if err := checkCmdSafe("npm.registry", n.Registry); err != nil {
			return err
		}
		publish = append(publish, "--registry", n.Registry)
	}
	b.add(PublishStage, Step{Name: "npm:publish", Command: shipconfig.Command{Workdir: b.workdir()}, Exec: publish})
	return nil
}

// cmdMetacharacters are the characters cmd.exe interprets even inside an
// argument: on Windows npm, pnpm and yarn are .cmd shims that run through
// cmd.exe, so a configured value holding one could run another command.
const cmdMetacharacters = "&|<>^%\"\r\n"

// checkCmdSafe refuses a value that reaches a .cmd shim's command line
// with a cmd.exe metacharacter, on every OS so a configuration behaves the
// same everywhere.
func checkCmdSafe(what, value string) error {
	if strings.ContainsAny(value, cmdMetacharacters) {
		return fmt.Errorf("%s %q contains a character cmd.exe interprets (& | < > ^ %% \" or a line break); npm runs through cmd.exe on Windows, so it is refused", what, value)
	}
	return nil
}
