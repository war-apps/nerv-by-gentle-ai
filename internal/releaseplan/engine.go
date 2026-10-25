package releaseplan

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

// VersionEngine is the release: section's version slot: its provider
// (release.version, else the slot's default), the provider's keys
// (release.providers.<provider>) and the projects it versions.
type VersionEngine struct {
	providers Providers
	provider  *extension.Provider
	config    map[string]any
	projects  []VersionProject
}

// NewVersionEngine resolves the version engine of rel (nil: no release:
// section, the default engine with its defaults, the whole repository as
// one project). An unknown provider or a key it does not declare is a
// *releaseconfig.ValidationError.
func NewVersionEngine(p Providers, rel *releaseconfig.Release) (*VersionEngine, error) {
	var name string
	var values map[string]any
	if rel != nil {
		name = rel.Version
	}
	prov, err := p.Provider(SlotVersion, name, releaseconfig.SectionKey+".version")
	if err != nil {
		return nil, err
	}
	var problems []string
	if rel != nil {
		problems = append(problems, unknownProviders(p, rel)...)
		values = rel.Providers[prov.Name]
	}
	cfg, more := providerConfig(prov, configLayer{at: releaseconfig.SectionKey + ".providers." + prov.Name, values: values})
	if problems = append(problems, more...); len(problems) > 0 {
		return nil, &releaseconfig.ValidationError{Problems: problems}
	}
	e := &VersionEngine{providers: p, provider: prov, config: cfg}
	if rel != nil {
		e.projects = VersionProjects(rel)
	}
	return e, nil
}

// unknownProviders names every release.providers key that is not a
// provider the tree ships.
func unknownProviders(p Providers, rel *releaseconfig.Release) []string {
	var problems []string
	shipped := append(p.SlotProviders(SlotVersion), p.SlotProviders(SlotBuild)...)
	check := func(at string, m map[string]map[string]any) {
		names := make([]string, 0, len(m))
		for name := range m {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			if !slices.Contains(shipped, name) {
				slices.Sort(shipped)
				problems = append(problems, fmt.Sprintf("%s.providers.%s: unknown release provider (want %s)", at, name, orList(shipped)))
			}
		}
	}
	check(releaseconfig.SectionKey, rel.Providers)
	for i, proj := range rel.Projects {
		check(fmt.Sprintf("%s.projects[%d]", releaseconfig.SectionKey, i), proj.Providers)
	}
	return problems
}

// Name is the engine's provider name.
func (e *VersionEngine) Name() string { return e.provider.Name }

// Config is the provider's resolved keys the engine is called with.
func (e *VersionEngine) Config() map[string]any { return e.config }

// Projects are the projects the engine versions (none: the whole
// repository).
func (e *VersionEngine) Projects() []VersionProject { return e.projects }

// VersionProjects are rel's projects as the version engine reads them.
func VersionProjects(rel *releaseconfig.Release) []VersionProject {
	out := []VersionProject{}
	for _, p := range rel.Projects {
		vp := VersionProject{Name: p.Name, Path: p.Path, SharedPaths: p.SharedPaths, Scopes: p.Scopes, TagFormat: p.TagFormat, Changelog: p.Changelog}
		for _, d := range p.DependsOn {
			vp.DependsOn = append(vp.DependsOn, VersionDependency{Project: d.Project, Bump: d.Bump})
		}
		out = append(out, vp)
	}
	return out
}

func (e *VersionEngine) payload() VersionPayload {
	return VersionPayload{Projects: e.projects}
}

// Validate has the engine check its keys and the projects before anything
// runs: its problems are a *releaseconfig.ValidationError.
func (e *VersionEngine) Validate(ctx context.Context) error {
	return e.providers.validate(ctx, e.provider, e.config, e.payload())
}

// Next computes the next release of each project (only those of only,
// with their dependencies, when not empty) on branch in the repository
// dir; with version, each requested project releases it instead.
func (e *VersionEngine) Next(ctx context.Context, dir, branch string, only []string, version string) ([]versioning.Result, error) {
	payload := e.payload()
	payload.Dir, payload.Branch, payload.Only, payload.Version = dir, branch, only, version
	var data NextData
	resp, err := e.providers.Call(ctx, e.provider, OpNext, e.config, payload, &data)
	if err != nil {
		return nil, answerError(resp, err)
	}
	return data.Results, nil
}

// Branch resolves branch against the engine's release branches in dir.
func (e *VersionEngine) Branch(ctx context.Context, dir, branch string) (versioning.ReleaseBranch, error) {
	payload := e.payload()
	payload.Dir, payload.Branch = dir, branch
	var data BranchData
	resp, err := e.providers.Call(ctx, e.provider, OpBranch, e.config, payload, &data)
	if err != nil {
		return versioning.ReleaseBranch{}, answerError(resp, err)
	}
	return versioning.ReleaseBranch{Name: data.Name, Prerelease: data.Prerelease, Channel: data.Channel}, nil
}

// Import reads the repository's own engine configuration in dir. A
// provider without import is ErrNotImplemented; the answer of a skipped or
// blocked import (no configuration, one to move by hand) is returned with
// an *ImportError.
func (e *VersionEngine) Import(ctx context.Context, dir string) (ImportData, error) {
	payload := e.payload()
	payload.Dir = dir
	var data ImportData
	resp, err := e.providers.Call(ctx, e.provider, OpImport, e.config, payload, &data)
	if err != nil {
		if errors.Is(err, ErrNotImplemented) {
			return ImportData{}, err
		}
		_ = resp.DecodeData(&data)
		if data.Manual != "" {
			return data, &ImportError{Message: resp.Message, Manual: data.Manual}
		}
		return data, answerError(resp, err)
	}
	if resp.Status == extension.StatusSkipped || data.Absent {
		return data, &ImportError{Message: resp.Message, Absent: true}
	}
	return data, nil
}

// ImportError is an import with nothing to write: no configuration
// (Absent) or one to move by hand (Manual names it).
type ImportError struct {
	Message string
	Absent  bool
	Manual  string
}

func (e *ImportError) Error() string { return e.Message }

// answerError is a provider's failure: its problems as a
// *releaseconfig.ValidationError when it answered them, its message when
// it gave one, else err.
func answerError(resp extension.Response, err error) error {
	var problems ProblemsData
	if resp.DecodeData(&problems) == nil && len(problems.Problems) > 0 {
		return &releaseconfig.ValidationError{Problems: problems.Problems}
	}
	if resp.Message != "" {
		return errors.New(resp.Message)
	}
	return err
}

// BuildProvider is one project's build slot: its provider and the
// provider's keys, resolved.
type BuildProvider struct {
	providers Providers
	provider  *extension.Provider
	config    map[string]any
	payload   BuildPayload
}

// NewBuildProvider resolves the build provider of rel's project i:
// release.projects[i].build, else release.build, with the provider's keys
// (release.providers.<provider>, overridden by
// release.projects[i].providers.<provider>). A missing or unknown provider
// or a key it does not declare is a *releaseconfig.ValidationError.
func NewBuildProvider(p Providers, rel *releaseconfig.Release, i int) (*BuildProvider, error) {
	proj := rel.Projects[i]
	at := fmt.Sprintf("%s.projects[%d]", releaseconfig.SectionKey, i)
	selectedAt := at + ".build"
	if proj.Build == "" {
		selectedAt = releaseconfig.SectionKey + ".build"
	}
	name := rel.BuildProvider(proj)
	if name == "" {
		return nil, &releaseconfig.ValidationError{Problems: []string{fmt.Sprintf("%s.build: required by nerv release (the build provider: %s; or set release.build for every project): there is no default", at, orList(p.SlotProviders(SlotBuild)))}}
	}
	prov, err := p.Provider(SlotBuild, name, selectedAt)
	if err != nil {
		return nil, err
	}
	cfg, problems := providerConfig(prov,
		configLayer{at: releaseconfig.SectionKey + ".providers." + prov.Name, values: rel.Providers[prov.Name]},
		configLayer{at: at + ".providers." + prov.Name, values: proj.Providers[prov.Name]})
	if len(problems) > 0 {
		return nil, &releaseconfig.ValidationError{Problems: problems}
	}
	var stages []string
	for _, stage := range releaseconfig.Stages {
		if _, ok := proj.Stages[stage]; ok {
			stages = append(stages, stage)
		}
	}
	if stages == nil {
		stages = []string{}
	}
	return &BuildProvider{providers: p, provider: prov, config: cfg, payload: BuildPayload{Project: proj.Name, At: at, Stages: stages}}, nil
}

// Name is the provider's name.
func (b *BuildProvider) Name() string { return b.provider.Name }

// Validate has the provider check its keys and the project's stages.
func (b *BuildProvider) Validate(ctx context.Context) error {
	return b.providers.validate(ctx, b.provider, b.config, b.payload)
}

// Plan asks the provider for the project's build steps.
func (b *BuildProvider) Plan(ctx context.Context) (BuildSteps, error) {
	var data PlanData
	resp, err := b.providers.Call(ctx, b.provider, OpPlan, b.config, b.payload, &data)
	if err != nil {
		return BuildSteps{}, answerError(resp, err)
	}
	return BuildSteps{Provider: b.provider.Name, PlanData: data}, nil
}

// BuildSteps is a project's build as its provider planned it.
type BuildSteps struct {
	// Provider is the build provider's name.
	Provider string
	PlanData
}
