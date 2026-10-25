package releaseplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig/yamlsection"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
	"gopkg.in/yaml.v3"
)

// The native release providers, by plugin name: the providers nerv
// implements itself, run as the hidden `nerv provider <plugin>
// <operation>` with the request on stdin and the response on stdout.
const (
	ConventionalCommits = "release-conventional-commits"
	DockerCompose       = "release-docker-compose"
	Bash                = "release-bash"
)

// NativeProviders are the native release providers' plugin names.
var NativeProviders = []string{Bash, ConventionalCommits, DockerCompose}

// NativeDeps are what the native providers run with.
type NativeDeps struct {
	// Git runs the version engine's git; nil means env.ExecRunner.
	Git env.Runner
}

// RunNative runs one operation of the native release provider plugin:
// the request on stdin, one JSON response on stdout, and the exit code
// (0 ok or skipped, 1 a failure the response explains, 2 a request that
// cannot be read).
func RunNative(ctx context.Context, plugin, op string, stdin io.Reader, stdout, stderr io.Writer, deps NativeDeps) int {
	req, err := extension.ReadRequest(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "nerv provider %s: %v\n", plugin, err)
		return 2
	}
	var resp extension.Response
	switch plugin {
	case ConventionalCommits:
		resp = conventionalCommits(ctx, op, req, deps)
	case DockerCompose, Bash:
		resp = buildProvider(plugin, op, req)
	default:
		fmt.Fprintf(stderr, "nerv provider: %q is not a native release provider\n", plugin)
		return 2
	}
	return extension.Answer(stdout, resp)
}

func ok(data any) extension.Response {
	raw, err := json.Marshal(data)
	if err != nil {
		return failed("%v", err)
	}
	return extension.Response{Status: extension.StatusOK, Data: raw}
}

func failed(format string, args ...any) extension.Response {
	return extension.Response{Status: extension.StatusFailed, Message: fmt.Sprintf(format, args...)}
}

func unknownOp(plugin, op string) extension.Response {
	return failed("%s has no operation %q", plugin, op)
}

// invalid answers a failed validate (or a refused config) with its
// problems.
func invalid(problems []string) extension.Response {
	r := ok(ProblemsData{Problems: problems})
	r.Status = extension.StatusFailed
	r.Message = "invalid release configuration:\n  " + strings.Join(problems, "\n  ")
	return r
}

// engineAt is where the conventional-commits keys live.
var engineAt = releaseconfig.SectionKey + ".providers." + strings.TrimPrefix(ConventionalCommits, Extension+"-")

// conventionalCommits is the conventional-commits version engine:
// internal/versioning, offline in the request's repository.
func conventionalCommits(ctx context.Context, op string, req extension.Request, deps NativeDeps) extension.Response {
	var payload VersionPayload
	if err := req.DecodePayload(&payload); err != nil {
		return failed("payload: %v", err)
	}
	if op == OpImport {
		return importSemanticRelease(payload.Dir)
	}
	cfg, problems := engineConfig(req, payload.Projects)
	if len(problems) > 0 {
		return invalid(problems)
	}
	r := versioning.Request{RepoDir: payload.Dir, Branch: payload.Branch, Projects: payload.Only, Config: cfg, Offline: true, Version: payload.Version, Runner: deps.Git}
	switch op {
	case OpValidate:
		return ok(ProblemsData{})
	case OpNext:
		results, err := versioning.Compute(ctx, r)
		if err != nil {
			return failed("%v", err)
		}
		if results == nil {
			results = []versioning.Result{}
		}
		return ok(NextData{Results: results})
	case OpBranch:
		b, err := versioning.BranchChannel(ctx, r)
		if err != nil {
			return failed("%v", err)
		}
		return ok(BranchData{Name: b.Name, Prerelease: b.Prerelease, Channel: b.Channel})
	}
	return unknownOp(ConventionalCommits, op)
}

// engineConfig decodes the engine's keys (the request's config) and the
// projects into a defaulted, validated versioning.Config; every problem is
// named by its path in .ai-workflow.yml.
func engineConfig(req extension.Request, projects []VersionProject) (versioning.Config, []string) {
	data, err := json.Marshal(req.Config)
	if err != nil {
		return versioning.Config{}, []string{engineAt + ": " + err.Error()}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return versioning.Config{}, []string{engineAt + ": " + err.Error()}
	}
	var c versioning.Config
	if len(doc.Content) == 1 {
		if err := doc.Content[0].Decode(&c); err != nil {
			return versioning.Config{}, yamlsection.DecodeProblems(engineAt, err)
		}
	}
	c.Projects = nil
	for _, p := range projects {
		deps := make([]versioning.Dependency, len(p.DependsOn))
		for i, d := range p.DependsOn {
			deps[i] = versioning.Dependency{Project: d.Project, Bump: d.Bump}
		}
		c.Projects = append(c.Projects, versioning.Project{
			Name: p.Name, Path: p.Path, SharedPaths: p.SharedPaths, Scopes: p.Scopes,
			TagFormat: p.TagFormat, Changelog: p.Changelog, DependsOn: deps,
		})
	}
	c = c.WithDefaults()
	var cerr *versioning.ConfigError
	if err := c.Validate(); errors.As(err, &cerr) {
		var problems []string
		for _, p := range cerr.Problems {
			if strings.HasPrefix(p, "projects") {
				problems = append(problems, releaseconfig.SectionKey+"."+p)
			} else {
				problems = append(problems, engineAt+"."+p)
			}
		}
		return versioning.Config{}, problems
	} else if err != nil {
		return versioning.Config{}, []string{engineAt + ": " + err.Error()}
	}
	return c, nil
}

// importSemanticRelease is conventional-commits' import: the
// repository's semantic-release configuration as the engine's keys.
func importSemanticRelease(dir string) extension.Response {
	imported, err := versioning.Import(dir)
	var manual *versioning.ManualMigrationError
	switch {
	case errors.As(err, &manual):
		r := ok(ImportData{Manual: manual.File})
		r.Status, r.Message = extension.StatusBlocked, err.Error()
		return r
	case errors.Is(err, versioning.ErrNoSemanticReleaseConfig):
		r := ok(ImportData{Absent: true})
		r.Status, r.Message = extension.StatusSkipped, err.Error()
		return r
	case err != nil:
		return failed("%v", err)
	}
	text, err := EngineYAML(imported.Config)
	if err != nil {
		return failed("%v", err)
	}
	data := ImportData{Source: imported.Source, Config: text}
	for _, w := range imported.Warnings {
		data.Warnings = append(data.Warnings, w.String())
	}
	return ok(data)
}

// EngineYAML renders the engine settings cfg sets (its projects aside) as
// the YAML mapping of the conventional-commits keys, in
// releaseconfig.MovedEngineKeys order; empty settings are left out.
func EngineYAML(cfg versioning.Config) (string, error) {
	values, err := engineValues(cfg)
	if err != nil {
		return "", err
	}
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, key := range releaseconfig.MovedEngineKeys {
		if v, ok := values[key]; ok {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
		}
	}
	if len(m.Content) == 0 {
		return "", nil
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// engineValues are cfg's engine settings as YAML nodes, keyed by their
// key; empty settings are left out.
func engineValues(cfg versioning.Config) (map[string]*yaml.Node, error) {
	values := map[string]*yaml.Node{}
	set := func(key string, v any) error {
		var n yaml.Node
		if err := n.Encode(v); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		values[key] = &n
		return nil
	}
	flow := func(pairs ...any) *yaml.Node {
		m := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
		for i := 0; i+1 < len(pairs); i += 2 {
			var n yaml.Node
			_ = n.Encode(pairs[i+1])
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: pairs[i].(string)}, &n)
		}
		return m
	}
	var branches []any
	for _, br := range cfg.Branches {
		if !br.Prerelease.IsSet() && br.Channel == "" {
			branches = append(branches, br.Name)
			continue
		}
		pairs := []any{"name", br.Name}
		switch {
		case br.Prerelease.UsesName():
			pairs = append(pairs, "prerelease", true)
		case br.Prerelease.ID() != "":
			pairs = append(pairs, "prerelease", br.Prerelease.ID())
		}
		if br.Channel != "" {
			pairs = append(pairs, "channel", br.Channel)
		}
		branches = append(branches, flow(pairs...))
	}
	if len(branches) > 0 {
		if err := set("branches", branches); err != nil {
			return nil, err
		}
	}
	if len(cfg.ReleaseRules) > 0 {
		var rules []any
		for _, r := range cfg.ReleaseRules {
			var pairs []any
			if r.Type != "" {
				pairs = append(pairs, "type", r.Type)
			}
			if r.Scope != "" {
				pairs = append(pairs, "scope", r.Scope)
			}
			if r.Breaking {
				pairs = append(pairs, "breaking", true)
			}
			if r.Revert {
				pairs = append(pairs, "revert", true)
			}
			rules = append(rules, flow(append(pairs, "release", r.Release)...))
		}
		if err := set("release_rules", rules); err != nil {
			return nil, err
		}
	}
	if len(cfg.NoteKeywords) > 0 {
		if err := set("note_keywords", cfg.NoteKeywords); err != nil {
			return nil, err
		}
	}
	for key, v := range map[string]string{"tag_format": cfg.TagFormat, "repository_url": cfg.RepositoryURL} {
		if v != "" {
			if err := set(key, v); err != nil {
				return nil, err
			}
		}
	}
	changelog := map[string]string{}
	if cfg.Changelog.File != "" {
		changelog["file"] = cfg.Changelog.File
	}
	if cfg.Changelog.Title != "" {
		changelog["title"] = cfg.Changelog.Title
	}
	if len(changelog) > 0 {
		if err := set("changelog", changelog); err != nil {
			return nil, err
		}
	}
	return values, nil
}

// composeOnlyStages are the stages a docker-compose project may still
// write commands for: compose runs build, test and package itself.
var composeOnlyStages = []string{"restore", "lint", "coverage"}

// sharedHostStages are the bash stages that get the build-server
// environment with shared_host: the stages dotnet commands run in.
var sharedHostStages = []string{"restore", "lint", "build", "test", "package"}

var serviceRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// buildConfig is the build providers' keys.
type buildConfig struct {
	File        string `json:"file"`
	TestService string `json:"test_service"`
	SharedHost  bool   `json:"shared_host"`
}

// buildProvider is docker-compose's or bash's validate and plan.
func buildProvider(plugin, op string, req extension.Request) extension.Response {
	var payload BuildPayload
	if err := req.DecodePayload(&payload); err != nil {
		return failed("payload: %v", err)
	}
	var cfg buildConfig
	if err := req.DecodeConfig(&cfg); err != nil {
		return failed("config: %v", err)
	}
	name := strings.TrimPrefix(plugin, Extension+"-")
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	keyAt := payload.At + ".providers." + name
	if plugin == DockerCompose {
		switch {
		case cfg.File == "":
			add("%s.file: required by the docker-compose build (or release.providers.docker-compose.file for every project)", keyAt)
		default:
			if p := yamlsection.PathProblem(keyAt+".file", cfg.File); p != "" {
				problems = append(problems, p)
			}
		}
		if cfg.TestService != "" && !serviceRe.MatchString(cfg.TestService) {
			add("%s.test_service: %q is not a compose service name", keyAt, cfg.TestService)
		}
		stages := slices.Clone(payload.Stages)
		sort.Strings(stages)
		for _, stage := range stages {
			if slices.Contains(releaseconfig.Stages, stage) && !slices.Contains(composeOnlyStages, stage) {
				add("%s.stages.%s: docker-compose runs %s itself (docker compose build, run and save); remove it, or use build: bash", payload.At, stage, stage)
			}
		}
	}
	if len(problems) > 0 {
		return invalid(problems)
	}
	switch op {
	case OpValidate:
		return ok(ProblemsData{})
	case OpPlan:
		if plugin == Bash {
			data := PlanData{ImagePrefix: "docker:"}
			if cfg.SharedHost {
				data.StageEnv = map[string]map[string]string{}
				for _, stage := range sharedHostStages {
					data.StageEnv[stage] = DotnetBuildServerEnv()
				}
			}
			return ok(data)
		}
		file := yamlsection.NormalizePath(cfg.File)
		data := PlanData{ImagePrefix: "compose:", Paths: map[string]string{"file": file}}
		data.Steps = append(data.Steps, BuildStep{Stage: "build", Name: "compose:build", Exec: []string{"docker", "compose", "-f", file, "build"}})
		if svc := cfg.TestService; svc != "" {
			data.Steps = append(data.Steps, BuildStep{Stage: "test", Name: "compose:test", Exec: []string{"docker", "compose", "-f", file, "run", "--rm", svc}})
		}
		return ok(data)
	}
	return unknownOp(plugin, op)
}
