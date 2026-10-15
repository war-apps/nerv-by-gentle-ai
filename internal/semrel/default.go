package semrel

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// Where the semantic-release configuration of a result came from
// (ProjectResult.SemanticReleaseConfig).
const (
	// ConfigRepository: the repository's own (.releaserc*, release.config.*,
	// package.json "release", ...), found as semantic-release finds it.
	ConfigRepository = "repository"
	// ConfigBuiltIn: NERV's built-in default, dim.comprobantes.api's
	// configuration (runner/nerv-semrel-lib.mjs defaultConfig), used when
	// the repository has none.
	ConfigBuiltIn = "default"
)

// ErrFixedPrereleaseGlob is wrapped when a branch pattern with a fixed
// prerelease id (release/* with rc) matches several branches: semantic-release
// would stop with EPRERELEASEBRANCHES (verified with semantic-release 24.2.3).
var ErrFixedPrereleaseGlob = errors.New("a fixed prerelease id matches several branches")

// defaultBranch is one branch of the built-in configuration, as the runner
// reads it.
type defaultBranch struct {
	Name       string `json:"name"`
	Prerelease string `json:"prerelease,omitempty"`
	Channel    string `json:"channel,omitempty"`
}

// localConfig is the user's local .nerv/nerv.yaml: its release: section and
// its pipeline: section, whose branches feed the built-in default (an
// invalid pipeline: section only matters when the default is used).
type localConfig struct {
	rel         *shipconfig.Release
	pipeline    *pipelineconfig.Pipeline
	pipelineErr error
}

// loadLocalConfig loads both sections of the .nerv/nerv.yaml in dir.
func loadLocalConfig(dir string) (localConfig, error) {
	rel, err := loadRelease(dir)
	if err != nil {
		return localConfig{}, err
	}
	lc := localConfig{rel: rel}
	lc.pipeline, lc.pipelineErr = pipelineconfig.Load(dir)
	if errors.Is(lc.pipelineErr, pipelineconfig.ErrNotConfigured) {
		lc.pipeline, lc.pipelineErr = nil, nil
	}
	return lc, nil
}

// defaultBranches are the built-in configuration's branches: the pipeline:
// section's, or its defaults (main, develop alpha, release/* rc on beta,
// hotfix/* beta) when there is no such section.
func (lc localConfig) defaultBranches() ([]defaultBranch, error) {
	if lc.pipelineErr != nil {
		return nil, fmt.Errorf("the built-in semantic-release configuration takes its branches from the pipeline: section: %w", lc.pipelineErr)
	}
	branches := pipelineconfig.DefaultBranches()
	if lc.pipeline != nil {
		branches = lc.pipeline.Branches
	}
	out := make([]defaultBranch, len(branches))
	for i, b := range branches {
		channel := b.Channel
		if channel == "" {
			channel = b.Prerelease
		}
		out[i] = defaultBranch{Name: b.Name, Prerelease: b.Prerelease, Channel: channel}
	}
	return out, nil
}

// checkFixedPrereleaseGlobs refuses a branch pattern with a fixed
// prerelease id that matches more than one of heads (branch names):
// semantic-release requires a different prerelease id per branch.
func checkFixedPrereleaseGlobs(branches []defaultBranch, heads []string) error {
	for _, b := range branches {
		if b.Prerelease == "" || !strings.ContainsAny(b.Name, "*?[") {
			continue
		}
		var matched []string
		for _, h := range heads {
			if ok, _ := path.Match(b.Name, h); ok {
				matched = append(matched, h)
			}
		}
		if len(matched) > 1 {
			slices.Sort(matched)
			return fmt.Errorf("%w: %s uses the prerelease id %q for every branch it matches, and origin has %s; semantic-release needs one id per prerelease branch (EPRERELEASEBRANCHES): keep one such branch at a time (merge or delete the others), or list them in the pipeline: section's branches with their own prerelease ids",
				ErrFixedPrereleaseGlob, b.Name, b.Prerelease, strings.Join(matched, ", "))
		}
	}
	return nil
}

// branchNames lists the branch names under prefix (refs/heads/ or
// refs/remotes/origin/) of the repository at dir, without HEAD.
func (e *Engine) branchNames(ctx context.Context, dir string, env []string, prefix string) ([]string, error) {
	out, err := e.git(ctx, dir, env, "for-each-ref", "--format=%(objectname) %(refname)", prefix)
	if err != nil {
		return nil, err
	}
	var names []string
	for line := range strings.Lines(out) {
		_, ref, ok := strings.Cut(strings.TrimSpace(line), " ")
		if name, found := strings.CutPrefix(ref, prefix); ok && found && name != "HEAD" {
			names = append(names, name)
		}
	}
	return names, nil
}

// configSource sets where r's semantic-release configuration came from.
func configSource(r *ProjectResult, configFile string) {
	if configFile == "" {
		r.SemanticReleaseConfig = ConfigBuiltIn
		return
	}
	r.SemanticReleaseConfig, r.SemanticReleaseConfigFile = ConfigRepository, configFile
}
