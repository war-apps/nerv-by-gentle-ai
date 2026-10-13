// Package semrel computes the next semantic version of a repository, or of
// each project of a monorepo declared in the release: section of
// .nerv/nerv.yaml, by running semantic-release itself in dry-run mode.
//
// semantic-release and its plugins are installed at pinned versions in a
// NERV cache (never in the repository) and driven by an embedded node runner
// (runner/nerv-semrel.mjs) through semantic-release's programmatic API, so
// every form of the repository's own configuration is honoured. The run
// happens in a worktree of a disposable mirror of the repository (with
// origin's release tags and notes), so the user's checkout and refs are never
// touched, and it needs no release credentials: the steps that
// verify tokens or have side effects are disabled for this read-only run
// (see the runner's header for the semantic-release sources relied on).
package semrel

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

//go:embed runner/nerv-semrel.mjs
var runnerScript []byte

// Options select what Compute computes.
type Options struct {
	// RepoRoot is any directory inside the repository.
	RepoRoot string
	// Branch is the local branch to compute; empty means the current one.
	Branch string
	// Project restricts the result to one project of the release: section
	// (its dependencies are still computed, to apply depends_on).
	Project string
	// Env is the environment of git and node; nil means os.Environ().
	Env []string
	// CacheDir is the base of the semantic-release cache; empty means
	// <user cache dir>/nerv/semantic-release.
	CacheDir string
	// Offline skips reading release tags and notes from the origin remote;
	// the local ones are used as they are (release channels may be missed:
	// a plain git fetch never brings semantic-release's notes).
	Offline bool
	// FetchTimeout bounds the read of release tags and notes from origin;
	// zero means DefaultFetchTimeout.
	FetchTimeout time.Duration
}

// Engine computes versions through a Runner.
type Engine struct {
	// Runner runs git, node and npm; nil means ExecRunner.
	Runner Runner
}

// Compute computes the next release of every project with the default
// engine. See Engine.Compute.
func Compute(ctx context.Context, opts Options) ([]ProjectResult, error) {
	return (&Engine{}).Compute(ctx, opts)
}

func (e *Engine) runner() Runner {
	if e.Runner == nil {
		return ExecRunner{}
	}
	return e.Runner
}

// Compute returns the next release of each project on the branch, in
// dependency order (only the requested project when opts.Project is set).
func (e *Engine) Compute(ctx context.Context, opts Options) ([]ProjectResult, error) {
	env := opts.Env
	if env == nil {
		env = os.Environ()
	}
	top, err := e.git(ctx, opts.RepoRoot, env, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	top = filepath.Clean(top)

	rel, err := shipconfig.Load(top)
	if errors.Is(err, shipconfig.ErrNotConfigured) {
		rel, err = nil, nil
	}
	if err != nil {
		return nil, err
	}
	projects, err := planProjects(rel, filepath.Base(top), opts.Project)
	if err != nil {
		return nil, err
	}
	if err := e.checkNode(ctx, env); err != nil {
		return nil, err
	}

	srVersion, extras := shipconfig.DefaultSemanticReleaseVersion, []string(nil)
	if rel != nil {
		srVersion, extras = rel.SemanticRelease.Version, rel.SemanticRelease.ExtraPlugins
	}
	specs := pinnedSpecs(srVersion, extras)
	base := opts.CacheDir
	if base == "" {
		if base, err = defaultCacheBase(); err != nil {
			return nil, err
		}
	}
	cacheDir, err := e.ensureCache(ctx, base, specs)
	if err != nil {
		return nil, err
	}

	branch := opts.Branch
	if branch == "" {
		if branch, err = e.git(ctx, top, env, "symbolic-ref", "--short", "HEAD"); err != nil {
			return nil, fmt.Errorf("the current checkout is not on a branch; pass a branch: %w", err)
		}
	}
	if _, err := e.git(ctx, top, env, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
		return nil, fmt.Errorf("branch %q: not a local branch: %w", branch, err)
	}
	remote := origin{Timeout: opts.FetchTimeout}
	if remote.Timeout <= 0 {
		remote.Timeout = DefaultFetchTimeout
	}
	if !opts.Offline {
		// No origin remote means there is nothing to sync from.
		url, _ := e.git(ctx, top, env, "remote", "get-url", "origin")
		remote.URL = resolveRemoteURL(url, top)
		if remote.URL != "" {
			remote.SSHCommand, _ = e.git(ctx, top, env, "config", "--get", "core.sshCommand")
		}
	}

	scriptDir, err := os.MkdirTemp("", "nerv-semrel-runner-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scriptDir)
	script := filepath.Join(scriptDir, "nerv-semrel.mjs")
	if err := os.WriteFile(script, runnerScript, 0o644); err != nil {
		return nil, err
	}

	var results []ProjectResult
	err = e.withCheckout(ctx, top, branch, remote, env, func(wt, mirror string) error {
		node := func(req runnerRequest) (runnerResponse, error) {
			req.CacheDir, req.Cwd = cacheDir, wt
			return e.node(ctx, script, wt, nodeEnv(env), req)
		}
		plan, err := node(runnerRequest{Mode: "plan"})
		if err != nil {
			return err
		}
		if err := checkPins(plan.Packages, plan.Unresolved, specs); err != nil {
			return err
		}
		byName := map[string]ProjectResult{}
		for _, p := range projects {
			resp, err := node(runnerRequest{
				Mode:           "run",
				RepositoryURL:  mirror,
				TagFormat:      p.TagFormat,
				Filter:         p.Filter,
				MinReleaseType: minReleaseType(p.DependsOn, byName),
			})
			if err != nil {
				return fmt.Errorf("project %s: %w", p.Name, err)
			}
			r := normalize(p.Name, branch, resp)
			byName[p.Name] = r
			if opts.Project == "" || p.Name == opts.Project {
				results = append(results, r)
			}
		}
		return nil
	})
	return results, err
}

// git runs git in dir and returns its trimmed stdout.
func (e *Engine) git(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	res, err := e.run(ctx, Command{Name: "git", Args: append([]string{"-C", dir}, args...), Env: env})
	return strings.TrimSpace(string(res.Stdout)), err
}

// withCheckout runs fn(worktree, mirror) on a disposable copy of the
// repository and always deletes it afterwards; the user's repository is only
// read (no worktree entry, ref, tag or note is added to it).
//
//   - mirror: `git clone --mirror --shared` of the repository, a bare
//     repository holding every local branch, tag and note, sharing the
//     objects (no copy). It is semantic-release's "remote" (see the runner).
//   - When remote.URL is set (origin's URL), the release tags and
//     semantic-release's git notes (refs/notes/semantic-release-<tag>, which
//     record the channel of each release) are fetched from it into the
//     mirror, origin winning, as semantic-release does in CI: a plain git
//     fetch never brings notes, and without them every prerelease tag looks
//     like a default-channel release, so the last prerelease is missed.
//     The fetch never prompts (see fetchEnv), never gets the URL on its
//     command line, and is bounded by remote.Timeout.
//   - worktree: a worktree of the mirror on branch (semantic-release needs a
//     named branch).
func (e *Engine) withCheckout(ctx context.Context, repo, branch string, remote origin, env []string, fn func(worktree, mirror string) error) error {
	tmp, err := os.MkdirTemp("", "nerv-semrel-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	mirror := filepath.Join(tmp, "repo.git")
	wt := filepath.Join(tmp, "worktree")

	if _, err := e.git(ctx, repo, env, "clone", "--quiet", "--mirror", "--shared", repo, mirror); err != nil {
		return fmt.Errorf("copying the repository: %w", err)
	}
	if remote.URL != "" {
		if err := e.fetchOrigin(ctx, mirror, remote, env); err != nil {
			return err
		}
	}
	if _, err := e.git(ctx, mirror, env, "worktree", "add", "--quiet", wt, branch); err != nil {
		return err
	}
	return fn(wt, mirror)
}

// fetchOrigin fetches origin's release tags and notes into the mirror.
func (e *Engine) fetchOrigin(ctx context.Context, mirror string, remote origin, env []string) error {
	fctx, cancel := context.WithTimeout(ctx, remote.Timeout)
	defer cancel()
	_, err := e.git(fctx, mirror, fetchEnv(env, remote), "fetch", "--quiet", "--no-write-fetch-head", originRemote,
		"+refs/tags/*:refs/tags/*", "+refs/notes/*:refs/notes/*")
	switch {
	case err == nil:
		return nil
	case errors.Is(fctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil:
		return fmt.Errorf("reading release tags and notes from origin (%s) timed out after %s; use offline mode to rely on the local tags only", redact(remote.URL), remote.Timeout)
	}
	return fmt.Errorf("reading release tags and notes from origin (%s; needed for release channels; use offline mode to rely on the local tags only): %w", redact(remote.URL), err)
}

// node runs the embedded runner with one request.
func (e *Engine) node(ctx context.Context, script, dir string, env []string, req runnerRequest) (runnerResponse, error) {
	stdin, err := json.Marshal(req)
	if err != nil {
		return runnerResponse{}, err
	}
	res, runErr := e.runner().Run(ctx, Command{Name: "node", Args: []string{script}, Dir: dir, Env: env, Stdin: stdin})
	resp, err := parseResponse(res.Stdout, res.Stderr)
	if err != nil {
		return resp, err
	}
	if runErr != nil {
		return resp, fmt.Errorf("semantic-release runner: %w%s", runErr, stderrTail(res.Stderr))
	}
	return resp, nil
}

// minNode is the oldest Node.js semantic-release 24 supports
// (engines.node ">=20.8.1").
var minNode = [3]int{20, 8, 1}

var nodeVersionRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

// nodeSupported reports whether a `node --version` output is at least minNode.
func nodeSupported(out string) (bool, error) {
	m := nodeVersionRe.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return false, fmt.Errorf("unrecognised node version %q", strings.TrimSpace(out))
	}
	for i := range 3 {
		n, _ := strconv.Atoi(m[i+1])
		if n != minNode[i] {
			return n > minNode[i], nil
		}
	}
	return true, nil
}

func (e *Engine) checkNode(ctx context.Context, env []string) error {
	res, err := e.runner().Run(ctx, Command{Name: "node", Args: []string{"--version"}, Env: env})
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("Node.js is required to compute versions with semantic-release (node was not found in PATH)")
		}
		return fmt.Errorf("node --version: %w%s", err, stderrTail(res.Stderr))
	}
	ok, err := nodeSupported(string(res.Stdout))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("Node.js %s is too old: semantic-release needs %d.%d.%d or newer", strings.TrimSpace(string(res.Stdout)), minNode[0], minNode[1], minNode[2])
	}
	return nil
}

// ciDetectionVars are the variables env-ci (semantic-release's CI detection,
// env-ci 11 services/*.js detect()) reads to recognise a CI service. They are
// removed from the runner's environment so semantic-release reads the branch
// from the worktree's git HEAD, never from a CI service's variables.
var ciDetectionVars = []string{
	"CI", "APPVEYOR", "BUILD_BUILDURI", "bamboo_agentId", "BITBUCKET_BUILD_NUMBER", "BITRISE_IO",
	"BUDDY_WORKSPACE_ID", "BUILDKITE", "CIRCLECI", "CIRRUS_CI", "CF_PAGES", "CODEBUILD_BUILD_ID",
	"CF_BUILD_ID", "CI_NAME", "DRONE", "GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_URL",
	"JB_SPACE_EXECUTION_NUMBER", "NETLIFY", "DISTELLI_APPNAME", "SAILCI", "SCREWDRIVER", "SCRUTINIZER",
	"SEMAPHORE", "SHIPPABLE", "TEAMCITY_VERSION", "TRAVIS", "VELA", "VERCEL", "NOW_GITHUB_DEPLOYMENT",
	"WERCKER_MAIN_PIPELINE_STARTED",
}

// nodeEnv is env without the CI detection variables.
func nodeEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		strip := false
		for _, v := range ciDetectionVars {
			if name == v {
				strip = true
				break
			}
		}
		if !strip {
			out = append(out, kv)
		}
	}
	return out
}
