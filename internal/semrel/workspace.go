package semrel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// ErrNoOrigin is returned by NewWorkspace for a repository without an origin
// remote: nerv ship releases what is on origin.
var ErrNoOrigin = errors.New("the repository has no origin remote")

// ErrVersionMismatch is wrapped by Workspace.Release when semantic-release
// would release (or released) another version than the one computed and
// confirmed before; the runner stops semantic-release at its verifyRelease
// step in that case, so nothing is tagged.
var ErrVersionMismatch = errors.New("semantic-release did not release the computed version")

// ErrPushDenied is wrapped by Workspace.CheckPush when git cannot push to
// origin with the user's own credentials.
var ErrPushDenied = errors.New("git cannot push to origin")

// WorkspaceOptions select the release clone NewWorkspace creates.
type WorkspaceOptions struct {
	// RepoRoot is any directory inside the user's repository.
	RepoRoot string
	// Branch is the branch to release. Required.
	Branch string
	// Env is the environment of git and node; nil means os.Environ().
	Env []string
	// CacheDir is the base of the semantic-release cache; empty means the
	// default (see Options.CacheDir).
	CacheDir string
	// FetchTimeout bounds each read of origin (the fetch, the push check, the
	// tag check); zero means DefaultFetchTimeout.
	FetchTimeout time.Duration
}

// Workspace is the release clone of nerv ship: a fresh clone of the user's
// repository in a temporary directory (never a worktree of it, so the
// user's repository is never modified), holding exactly origin's branches,
// tags and semantic-release notes, checked out on origin's commit of the
// branch. Its origin remote is origin's push address without credentials;
// the real URL, when it differs (it carries credentials), is written only
// into the clone's own .git/config as url.<real>.insteadOf (owner-only file
// in an owner-only temporary directory), so neither a command line nor an
// environment ever holds it. The release: section is the user's local one
// (their working tree's .nerv/nerv.yaml, committed or not, loaded once by
// NewWorkspace), never the clone's copy: only the code and the
// semantic-release configuration come from origin. Close removes
// everything.
type Workspace struct {
	e    *Engine
	root string
	// top is the user's repository and userEnv the user's environment (as
	// given, without the prompt-free settings), read for the commit
	// identity.
	top     string
	userEnv []string
	dir     string
	branch  string
	env     []string
	pushURL string
	public  string
	head    string
	cache   string
	timeout time.Duration
	// warnedChangelog: the ignored-changelog warning was printed.
	warnedChangelog bool
	// rel is the user's local release: section; nil when there is none.
	local localConfig
}

// NewWorkspace creates a release clone with the default engine. See
// Engine.NewWorkspace.
func NewWorkspace(ctx context.Context, opts WorkspaceOptions) (*Workspace, error) {
	return (&Engine{}).NewWorkspace(ctx, opts)
}

// NewWorkspace clones the user's repository (sharing its objects, without
// tags or a checkout) into a new temporary directory, points the clone's
// origin at the user's origin, fetches origin's branches (pruning the
// user's local ones), tags and notes, and checks the branch out at
// origin's commit when origin has it (OriginHead is empty otherwise). A
// repository without origin gives ErrNoOrigin; an unreadable origin wraps
// ErrOriginFetch. On error nothing is left behind.
func (e *Engine) NewWorkspace(ctx context.Context, opts WorkspaceOptions) (*Workspace, error) {
	if opts.Branch == "" {
		return nil, errors.New("release clone: a branch is required")
	}
	env := opts.Env
	if env == nil {
		env = os.Environ()
	}
	timeout := opts.FetchTimeout
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}
	top, err := e.git(ctx, opts.RepoRoot, env, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	top = filepath.Clean(top)
	local, err := loadLocalConfig(top)
	if err != nil {
		return nil, err
	}
	remotes, err := e.git(ctx, top, env, "remote")
	if err != nil {
		return nil, fmt.Errorf("listing the repository's remotes: %w", err)
	}
	if !slices.Contains(strings.Fields(remotes), "origin") {
		return nil, ErrNoOrigin
	}
	raw, err := e.git(ctx, top, env, "remote", "get-url", "origin")
	if err != nil {
		return nil, fmt.Errorf("reading origin's URL: %w", err)
	}
	if raw == "" {
		return nil, ErrNoOrigin
	}
	resolved := resolveRemoteURL(raw, top)
	pushURL, err := pushAddress(raw, resolved)
	if err != nil {
		return nil, err
	}
	sshCommand, _ := e.git(ctx, top, env, "config", "--get", "core.sshCommand")
	wenv := promptFreeEnv(env, sshCommand)

	root, err := os.MkdirTemp("", "nerv-ship-")
	if err != nil {
		return nil, err
	}
	w := &Workspace{e: e, root: root, top: top, userEnv: env, dir: filepath.Join(root, filepath.Base(top)), branch: opts.Branch, env: wenv,
		pushURL: pushURL, public: publicURL(raw), cache: opts.CacheDir, timeout: timeout, local: local}
	if err := w.init(ctx, top, env, resolved); err != nil {
		os.RemoveAll(root)
		return nil, err
	}
	return w, nil
}

func (w *Workspace) init(ctx context.Context, top string, env []string, realURL string) error {
	for _, d := range []string{w.ArtifactsDir(), w.NotesDir()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	e := w.e
	if _, err := e.git(ctx, top, env, "clone", "--quiet", "--shared", "--no-checkout", "--no-tags", top, w.dir); err != nil {
		return fmt.Errorf("cloning the repository for the release: %w", err)
	}
	if _, err := e.git(ctx, w.dir, env, "remote", "set-url", "origin", w.pushURL); err != nil {
		return fmt.Errorf("setting the release clone's origin: %w", err)
	}
	if realURL != w.pushURL {
		if err := writeOriginRewrite(filepath.Join(w.dir, ".git", "config"), realURL, w.pushURL); err != nil {
			return fmt.Errorf("configuring the release clone's origin: %w", err)
		}
	}
	fctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	_, err := e.git(fctx, w.dir, w.env, "fetch", "--quiet", "--prune", "--no-write-fetch-head", "origin",
		"+refs/heads/*:refs/remotes/origin/*", "+refs/tags/*:refs/tags/*", "+refs/notes/*:refs/notes/*")
	switch {
	case err == nil:
	case errors.Is(fctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil:
		return fmt.Errorf("reading origin (%s) timed out after %s: %w", redact(w.pushURL), w.timeout, ErrOriginFetch)
	default:
		return fmt.Errorf("reading origin (%s): %w: %w", redact(w.pushURL), ErrOriginFetch, err)
	}
	ref := "refs/remotes/origin/" + w.branch
	// rev-parse --verify --quiet exits 1 when origin has no such branch.
	if head, err := e.git(ctx, w.dir, env, "rev-parse", "--verify", "--quiet", ref); err == nil {
		w.head = head
	}
	if w.head == "" {
		return nil
	}
	if _, err := e.git(ctx, w.dir, env, "checkout", "--quiet", "--no-track", "-B", w.branch, ref); err != nil {
		return fmt.Errorf("checking out origin's %s: %w", w.branch, err)
	}
	return nil
}

// Dir is the clone's top-level directory, named like the user's repository.
func (w *Workspace) Dir() string { return w.dir }

// ArtifactsDir is a directory outside the clone for build outputs.
func (w *Workspace) ArtifactsDir() string { return filepath.Join(w.root, "artifacts") }

// NotesDir is a directory outside the clone for release-notes files.
func (w *Workspace) NotesDir() string { return filepath.Join(w.root, "notes") }

// OriginHead is origin's commit of the branch; empty when origin has no such
// branch (nothing is checked out then).
func (w *Workspace) OriginHead() string { return w.head }

// PublicURL is origin's address without credentials, for links; empty for a
// local origin.
func (w *Workspace) PublicURL() string { return w.public }

// Config is the release: section of the user's local .nerv/nerv.yaml (nil
// when it has none), the one every step of the release uses; the clone's
// copy is ignored.
func (w *Workspace) Config() *shipconfig.Release { return w.local.rel }

// Close removes the clone and every file the workspace wrote.
func (w *Workspace) Close() error { return os.RemoveAll(w.root) }

// Compute computes the next release of each project (only project, with its
// dependencies, when not empty) from the clone, as Compute does, with the
// user's local release: section (Config). The clone already holds origin's
// branches, tags and notes, so origin is not read again.
func (w *Workspace) Compute(ctx context.Context, project string) ([]ProjectResult, error) {
	return w.e.compute(ctx, Options{RepoRoot: w.dir, Branch: w.branch, Project: project, Env: w.env, CacheDir: w.cache, Offline: true},
		&w.local)
}

// CheckPush checks, as semantic-release does before releasing, that git can
// push the branch to origin with the user's own credentials (ssh keys, a
// credential helper such as `gh auth setup-git`). Checking first keeps
// semantic-release from falling back to a token embedded in the push URL,
// which would put it on git's command line.
func (w *Workspace) CheckPush(ctx context.Context) error {
	fctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	if _, err := w.e.git(fctx, w.dir, w.env, "push", "--dry-run", "--no-verify", "--quiet", "origin", "HEAD:refs/heads/"+w.branch); err != nil {
		return fmt.Errorf("%w (%s) with your credentials; set up ssh keys or a credential helper (for GitHub over https: gh auth setup-git): %w", ErrPushDenied, redact(w.pushURL), err)
	}
	return nil
}

// TagOnOrigin reports whether origin has the tag.
func (w *Workspace) TagOnOrigin(ctx context.Context, tag string) (bool, error) {
	fctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	out, err := w.e.git(fctx, w.dir, w.env, "ls-remote", "--tags", "origin", "refs/tags/"+tag)
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// BranchOnOrigin returns origin's current commit of the branch ("" when
// origin no longer has it), read now: a release commit pushed during the run
// moves it past OriginHead.
func (w *Workspace) BranchOnOrigin(ctx context.Context) (string, error) {
	fctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	out, err := w.e.git(fctx, w.dir, w.env, "ls-remote", "--heads", "origin", "refs/heads/"+w.branch)
	if err != nil {
		return "", err
	}
	sha, _, _ := strings.Cut(out, "\t")
	return strings.TrimSpace(sha), nil
}

// ErrTagNotOnOrigin is wrapped by CheckoutTag when origin has no such tag.
var ErrTagNotOnOrigin = errors.New("the tag is not on origin")

// CheckoutTag checks the clone out, detached, at origin's tag (the clone
// holds exactly origin's tags) and returns the tagged commit: the docker
// step builds a release from its released commit.
func (w *Workspace) CheckoutTag(ctx context.Context, tag string) (string, error) {
	commit, err := w.e.git(ctx, w.dir, w.env, "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}")
	// rev-parse --verify --quiet exits 1, printing nothing, for a ref that
	// does not exist; any other failure (a timeout, a broken clone, git
	// itself) is an environment error with its cause.
	var coder interface{ ExitCode() int }
	switch {
	case err == nil && commit != "":
	case err != nil && errors.As(err, &coder) && coder.ExitCode() == 1 && commit == "":
		return "", fmt.Errorf("tag %s: %w", tag, ErrTagNotOnOrigin)
	case err != nil:
		return "", fmt.Errorf("reading tag %s: %w", tag, err)
	default:
		return "", fmt.Errorf("tag %s: %w", tag, ErrTagNotOnOrigin)
	}
	if _, err := w.e.git(ctx, w.dir, w.env, "checkout", "--quiet", "--detach", commit); err != nil {
		return "", fmt.Errorf("checking out %s: %w", tag, err)
	}
	return commit, nil
}

// ReleaseRequest is one real semantic-release run in a Workspace.
type ReleaseRequest struct {
	// Expected is the project's computed result: its project name selects
	// the project and its version is the only one semantic-release may
	// release.
	Expected ProjectResult
	// Computed are every project's computed results, for depends_on.
	Computed []ProjectResult
	// Env is added to the runner's environment (GH_TOKEN, ...); it is never
	// put on a command line.
	Env []string
	// Output receives semantic-release's log; nil discards it.
	Output io.Writer
	// ExtraAssets are repository-relative files the release commit must hold
	// besides the changelog (version files written in the clone). Applied to
	// the built-in configuration's @semantic-release/git only: a
	// repository's own configuration lists its own assets.
	ExtraAssets []string
}

// Release runs semantic-release for real in the clone for one project, with
// the repository's own configuration and plugins (no dry run, ci false; the
// project's tagFormat, commit filter and depends_on raise as in Compute),
// pushing to origin. semantic-release is stopped at its verifyRelease step,
// before any tag, when its next version is not req.Expected's
// (ErrVersionMismatch); a release of another version, or none, is
// ErrVersionMismatch too.
func (w *Workspace) Release(ctx context.Context, req ReleaseRequest) (ProjectResult, error) {
	e := w.e
	rel := w.local.rel
	projects, err := planProjects(rel, filepath.Base(w.dir), "")
	if err != nil {
		return ProjectResult{}, err
	}
	var p *project
	for i := range projects {
		if projects[i].Name == req.Expected.Project {
			p = &projects[i]
		}
	}
	if p == nil {
		return ProjectResult{}, fmt.Errorf("project %q is not in the release: section", req.Expected.Project)
	}
	if !req.Expected.NewReleasePublished || req.Expected.NewReleaseVersion == "" {
		return ProjectResult{}, fmt.Errorf("project %s: no computed release to make", p.Name)
	}
	env := append(append([]string(nil), w.env...), req.Env...)
	cacheDir, specs, err := e.prepareNode(ctx, env, rel, []project{*p}, w.cache)
	if err != nil {
		return ProjectResult{}, err
	}
	scriptDir, err := os.MkdirTemp(w.root, "runner-")
	if err != nil {
		return ProjectResult{}, err
	}
	defer os.RemoveAll(scriptDir)
	if err := writeRunner(scriptDir); err != nil {
		return ProjectResult{}, err
	}
	script := filepath.Join(scriptDir, runnerMain)
	plan, err := e.checkPlan(ctx, script, w.dir, env, cacheDir, specs)
	if err != nil {
		return ProjectResult{}, err
	}
	configFile := plan.ConfigFile
	var defaults []defaultBranch
	if configFile == "" {
		if defaults, err = w.local.defaultBranches(); err != nil {
			return ProjectResult{}, err
		}
		if err := e.checkPrereleaseBranches(ctx, w.dir, env, "refs/remotes/origin/", defaults); err != nil {
			return ProjectResult{}, err
		}
	}
	byName := map[string]ProjectResult{}
	for _, r := range req.Computed {
		byName[r.Project] = r
	}
	expected := req.Expected.NewReleaseVersion
	resp, err := e.node(ctx, script, w.dir, nodeEnv(env), runnerRequest{
		Mode:             "release",
		CacheDir:         cacheDir,
		Cwd:              w.dir,
		RepositoryURL:    w.pushURL,
		TagFormat:        p.TagFormat,
		Filter:           p.Filter,
		MinReleaseType:   minReleaseType(p.DependsOn, byName),
		ExpectedVersion:  expected,
		Changelog:        p.Changelog,
		UseDefault:       configFile == "",
		Branches:         defaults,
		GitHubRelease:    remoteIsGitHub(w.public, env),
		ExtraAssets:      extraAssets(configFile, req.ExtraAssets),
		CommitFilesCache: filepath.Join(w.root, "commit-files.json"),
	}, req.Output)
	if m := resp.VersionMismatch; m != nil {
		return ProjectResult{}, fmt.Errorf("project %s: %w: semantic-release would release %s, computed and confirmed %s; stopped before the tag, nothing was released",
			p.Name, ErrVersionMismatch, m.Actual, m.Expected)
	}
	if err != nil {
		return ProjectResult{}, fmt.Errorf("project %s: %w", p.Name, err)
	}
	if resp.ChangelogIgnored && !w.warnedChangelog && req.Output != nil {
		w.warnedChangelog = true
		fmt.Fprintf(req.Output, "nerv ship: warning: the release: section sets each project's changelog, but the semantic-release configuration has no @semantic-release/changelog plugin: no changelog is written (add the plugin, and @semantic-release/git to commit it)\n")
	}
	r := normalize(p.Name, w.branch, resp)
	configSource(&r, plan)
	if !r.NewReleasePublished {
		return r, fmt.Errorf("project %s: %w: computed %s, semantic-release released nothing (see its log); nothing was tagged", p.Name, ErrVersionMismatch, expected)
	}
	if r.NewReleaseVersion != expected {
		return r, fmt.Errorf("project %s: %w: computed %s, semantic-release released %s (tag %s)", p.Name, ErrVersionMismatch, expected, r.NewReleaseVersion, r.NewReleaseGitTag)
	}
	return r, nil
}

// writeOriginRewrite appends url.<realURL>.insteadOf = <pushURL> to the git
// config file at path and makes it owner-only: git (and semantic-release's
// git) then reaches origin's real URL, with the credentials it holds, while
// every command line and environment only shows pushURL.
func writeOriginRewrite(path, realURL, pushURL string) error {
	if strings.ContainsAny(realURL+pushURL, "\n\r\x00") {
		return errors.New("origin's URL contains a line break")
	}
	quote := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	_, werr := fmt.Fprintf(f, "[url \"%s\"]\n\tinsteadOf = \"%s\"\n", quote.Replace(realURL), quote.Replace(pushURL))
	if err := f.Close(); werr == nil {
		werr = err
	}
	if werr != nil {
		return werr
	}
	return os.Chmod(path, 0o600)
}

// pushAddress is the address semantic-release pushes to and names: origin's
// URL without credentials (publicURL), or, for a local origin, its resolved
// path. Other addresses cannot be used.
func pushAddress(raw, resolved string) (string, error) {
	if public := publicURL(raw); public != "" {
		return public, nil
	}
	if isLocalRemote(raw) {
		return resolved, nil
	}
	return "", fmt.Errorf("origin's URL (%s) is neither a network address (https, ssh, git) nor a local path: nerv ship cannot release to it", redact(raw))
}

// isLocalRemote reports whether a remote URL is a local repository: a path
// or a file: URL.
func isLocalRemote(raw string) bool {
	switch {
	case strings.HasPrefix(strings.ToLower(raw), "file:"), driveRe.MatchString(raw), filepath.IsAbs(raw):
		return true
	case urlSchemeRe.MatchString(raw), scpLikeRe.MatchString(raw):
		return false
	}
	return raw != ""
}

// ReleaseURL is the release page of tag in the repository at public (a
// publicURL), https://<host>/<owner>/<repo>/releases/tag/<tag>, as
// dim.comprobantes.api's bump-csproj.sh writes it: ssh and git addresses
// become https (without the ssh port); http(s) keep their scheme and port.
// Empty when there is no public address or no tag.
func ReleaseURL(public, tag string) string {
	if public == "" || tag == "" {
		return ""
	}
	scheme, host, path := "https", "", ""
	switch {
	case urlSchemeRe.MatchString(public):
		u, err := url.Parse(public)
		if err != nil {
			return ""
		}
		host, path = u.Hostname(), u.Path
		if s := strings.ToLower(u.Scheme); s == "http" || s == "https" {
			scheme, host = s, u.Host
		}
	case scpLikeRe.MatchString(public):
		h, p, _ := strings.Cut(public, ":")
		if i := strings.LastIndex(h, "@"); i >= 0 {
			h = h[i+1:]
		}
		host, path = h, p
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" {
		return ""
	}
	return scheme + "://" + host + "/" + path + "/releases/tag/" + strings.ReplaceAll(url.PathEscape(tag), "%2F", "/")
}
