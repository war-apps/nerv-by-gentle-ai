package semrel

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
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
			ok, err := path.Match(b.Name, h)
			if err != nil {
				return fmt.Errorf("branch pattern %q: %w", b.Name, err)
			}
			if ok {
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

// checkPrereleaseBranches applies checkFixedPrereleaseGlobs to origin's
// branch set as semantic-release sees it, read from dir under prefix: the
// Compute mirror's refs/heads/ (set to origin's branches, see planHeads) or
// a release clone's refs/remotes/origin/ (fetched from origin with prune);
// both hold origin's branches as last read.
func (e *Engine) checkPrereleaseBranches(ctx context.Context, dir string, env []string, prefix string, branches []defaultBranch) error {
	heads, err := e.branchNames(ctx, dir, env, prefix)
	if err != nil {
		return err
	}
	return checkFixedPrereleaseGlobs(branches, heads)
}

// remoteIsGitHub reports whether public (a publicURL) is on GitHub:
// github.com, or the GitHub Enterprise host named by GH_HOST in env.
func remoteIsGitHub(public string, env []string) bool {
	host := ""
	switch {
	case urlSchemeRe.MatchString(public):
		if u, err := url.Parse(public); err == nil {
			host = u.Hostname()
		}
	case scpLikeRe.MatchString(public):
		h, _, _ := strings.Cut(public, ":")
		if i := strings.LastIndex(h, "@"); i >= 0 {
			h = h[i+1:]
		}
		host = h
	}
	host = strings.ToLower(host)
	if host == "" {
		return false
	}
	if host == "github.com" {
		return true
	}
	gh, _ := lookupEnv(env, "GH_HOST")
	return gh != "" && host == strings.ToLower(gh)
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

// extraAssets are the assets to add to the built-in configuration's git
// plugin: none when the repository has its own configuration.
func extraAssets(configFile string, assets []string) []string {
	if configFile != "" || len(assets) == 0 {
		return nil
	}
	return assets
}

// configSource sets where r's semantic-release configuration came from.
func configSource(r *ProjectResult, plan planInfo) {
	if plan.ConfigFile == "" {
		r.SemanticReleaseConfig = ConfigBuiltIn
		return
	}
	r.SemanticReleaseConfig, r.SemanticReleaseConfigFile = ConfigRepository, plan.ConfigFile
	r.GitPlugin, r.GitAssets = plan.GitPlugin, plan.GitAssets
}

// AssetsCover reports whether @semantic-release/git assets (globby
// patterns, as the plugin matches them against repository-relative paths)
// commit file: a pattern matches it (* and ? within a path segment, ** any
// segments, [...] classes, {a,b} alternatives; a leading ./ ignored) and no
// later "!" pattern excludes it.
func AssetsCover(assets []string, file string) bool {
	file = strings.TrimPrefix(file, "./")
	covered := false
	for _, a := range assets {
		negate := strings.HasPrefix(a, "!")
		pattern := strings.TrimPrefix(strings.TrimPrefix(a, "!"), "./")
		re, err := globRegexp(pattern)
		if err != nil || !re.MatchString(file) {
			continue
		}
		covered = !negate
	}
	return covered
}

// globRegexp compiles a globby pattern to an anchored regular expression.
func globRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	depth := 0
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:[^/]*/)*")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '[':
			end := strings.IndexByte(pattern[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("unclosed [ in %q", pattern)
			}
			b.WriteString(pattern[i : i+end+1])
			i += end
		case c == '{':
			depth++
			b.WriteString("(?:")
		case c == '}' && depth > 0:
			depth--
			b.WriteString(")")
		case c == ',' && depth > 0:
			b.WriteString("|")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
