// Package shipclone makes the throwaway clone of origin that `nerv ship`
// builds in and `nerv version next` computes in: a fresh clone of the user's
// repository in a temporary directory (never a worktree of it, so the user's
// repository is never written), holding exactly origin's branches, tags and
// semantic-release channel notes (refs/notes/semantic-release-<tag>, which
// the native version engine reads for prerelease numbering). The version is
// computed in the clone with internal/versioning, offline: origin was already
// read into it.
package shipclone

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

// DefaultFetchTimeout bounds each read of origin when Options.FetchTimeout
// is zero.
const DefaultFetchTimeout = 60 * time.Second

// ErrNoOrigin is returned by New for a repository without an origin
// remote: nerv ship builds what is on origin.
var ErrNoOrigin = errors.New("the repository has no origin remote")

// ErrOriginFetch is wrapped when origin's branches, release tags and notes
// cannot be read (unreachable, refused or timed out); callers can offer
// offline mode as the way out.
var ErrOriginFetch = errors.New("origin's branches, release tags and notes could not be read")

// Options select the clone New creates.
type Options struct {
	// RepoRoot is any directory inside the user's repository.
	RepoRoot string
	// Branch is the branch to build. Required.
	Branch string
	// Env is the environment of git; nil means os.Environ().
	Env []string
	// FetchTimeout bounds each read of origin (the fetch, the branch
	// check); zero means DefaultFetchTimeout.
	FetchTimeout time.Duration
	// Runner runs git; nil means proc.ExecRunner.
	Runner proc.Runner
	// VersionRunner runs the version engine's git in the clone; nil means
	// env.ExecRunner.
	VersionRunner env.Runner
	// LocalHead, when set, is the commit the clone's Branch is set to (the
	// user's local branch, for nerv version next) instead of checking out
	// origin's: nothing is checked out then.
	LocalHead string
}

// Workspace is the clone. Its origin remote is origin's push address
// without credentials; the real URL, when it differs (it carries
// credentials), is written only into the clone's own .git/config as
// url.<real>.insteadOf (owner-only file in an owner-only temporary
// directory), so neither a command line nor an environment ever holds it.
// The ship: section is the user's local one (their working tree's
// .ai-workflow.yml, committed or not, loaded once by New), never the clone's
// copy: only the code comes from origin. Close removes everything.
type Workspace struct {
	runner   proc.Runner
	versions env.Runner
	root     string
	dir      string
	branch   string
	env      []string
	pushURL  string
	public   string
	head     string
	timeout  time.Duration
	// rel is the user's local ship: section; nil when there is none.
	rel *shipconfig.Release
}

// New clones the user's repository (sharing its objects, without tags or a
// checkout) into a new temporary directory, points the clone's origin at the
// user's origin, fetches origin's branches (pruning the user's local ones),
// tags and notes, and checks the branch out at origin's commit when origin
// has it (OriginHead is empty otherwise). A repository without origin gives
// ErrNoOrigin; an unreadable origin wraps ErrOriginFetch; an invalid local
// ship: section stops it before anything is cloned. On error nothing is
// left behind.
func New(ctx context.Context, opts Options) (*Workspace, error) {
	if opts.Branch == "" {
		return nil, errors.New("release clone: a branch is required")
	}
	envv := opts.Env
	if envv == nil {
		envv = os.Environ()
	}
	timeout := opts.FetchTimeout
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}
	w := &Workspace{runner: opts.Runner, versions: opts.VersionRunner, branch: opts.Branch, timeout: timeout}
	if w.runner == nil {
		w.runner = proc.ExecRunner{}
	}
	if w.versions == nil {
		w.versions = env.ExecRunner{}
	}
	top, err := w.git(ctx, opts.RepoRoot, envv, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	top = filepath.Clean(top)
	rel, err := shipconfig.Load(top)
	switch {
	case errors.Is(err, shipconfig.ErrNotConfigured):
	case err != nil:
		return nil, err
	default:
		w.rel = rel
	}
	remotes, err := w.git(ctx, top, envv, "remote")
	if err != nil {
		return nil, fmt.Errorf("listing the repository's remotes: %w", err)
	}
	if !slices.Contains(strings.Fields(remotes), "origin") {
		return nil, ErrNoOrigin
	}
	raw, err := w.git(ctx, top, envv, "remote", "get-url", "origin")
	if err != nil {
		return nil, fmt.Errorf("reading origin's URL: %w", err)
	}
	if raw == "" {
		return nil, ErrNoOrigin
	}
	resolved := resolveRemoteURL(raw, top)
	if w.pushURL, err = pushAddress(raw, resolved); err != nil {
		return nil, err
	}
	w.public = publicURL(raw)
	sshCommand, _ := w.git(ctx, top, envv, "config", "--get", "core.sshCommand")
	w.env = promptFreeEnv(envv, sshCommand)

	if w.root, err = os.MkdirTemp("", "nerv-ship-"); err != nil {
		return nil, err
	}
	w.dir = filepath.Join(w.root, filepath.Base(top))
	if err := w.init(ctx, top, envv, resolved, opts.LocalHead); err != nil {
		os.RemoveAll(w.root)
		return nil, err
	}
	return w, nil
}

func (w *Workspace) init(ctx context.Context, top string, envv []string, realURL, localHead string) error {
	for _, d := range []string{w.ArtifactsDir(), w.NotesDir()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	if _, err := w.git(ctx, top, envv, "clone", "--quiet", "--shared", "--no-checkout", "--no-tags", top, w.dir); err != nil {
		return fmt.Errorf("cloning the repository for the release: %w", err)
	}
	if _, err := w.git(ctx, w.dir, envv, "remote", "set-url", "origin", w.pushURL); err != nil {
		return fmt.Errorf("setting the release clone's origin: %w", err)
	}
	if realURL != w.pushURL {
		if err := writeOriginRewrite(filepath.Join(w.dir, ".git", "config"), realURL, w.pushURL); err != nil {
			return fmt.Errorf("configuring the release clone's origin: %w", err)
		}
	}
	fctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	_, err := w.git(fctx, w.dir, w.env, "fetch", "--quiet", "--prune", "--no-write-fetch-head", "origin",
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
	if head, err := w.git(ctx, w.dir, envv, "rev-parse", "--verify", "--quiet", ref); err == nil {
		w.head = head
	}
	if localHead != "" {
		// The user's branch, as it is locally (its unpushed commits count),
		// among origin's branches; nothing is checked out.
		if _, err := w.git(ctx, w.dir, envv, "update-ref", "refs/heads/"+w.branch, localHead); err != nil {
			return fmt.Errorf("setting %s in the copy: %w", w.branch, err)
		}
		return nil
	}
	if w.head == "" {
		return nil
	}
	if _, err := w.git(ctx, w.dir, envv, "checkout", "--quiet", "--no-track", "-B", w.branch, ref); err != nil {
		return fmt.Errorf("checking out origin's %s: %w", w.branch, err)
	}
	return nil
}

// git runs git in dir and returns its trimmed stdout; a failure carries the
// redacted end of stderr.
func (w *Workspace) git(ctx context.Context, dir string, envv []string, args ...string) (string, error) {
	c := proc.Command{Name: "git", Args: append([]string{"-C", dir}, args...), Env: envv}
	res, err := w.runner.Run(ctx, c)
	if err != nil {
		return strings.TrimSpace(string(res.Stdout)), fmt.Errorf("%s: %w%s", redact(c.String()), err, stderrTail(res.Stderr))
	}
	return strings.TrimSpace(string(res.Stdout)), nil
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

// Config is the ship: section of the user's local .ai-workflow.yml (nil when
// it has none), the one every step uses; the clone's copy is ignored.
func (w *Workspace) Config() *shipconfig.Release { return w.rel }

// Close removes the clone and every file the workspace wrote.
func (w *Workspace) Close() error { return os.RemoveAll(w.root) }

// VersionConfig is the version engine's configuration: the local ship:
// section's, or the defaults when there is none.
func (w *Workspace) VersionConfig() versioning.Config {
	if w.rel == nil {
		return versioning.Config{}.WithDefaults()
	}
	return w.rel.Versioning()
}

// Compute computes the next release of each project (only projects, with
// their dependencies, when not empty) in the clone with the native engine,
// offline: the clone already holds origin's branches, tags and notes, so
// origin is not read again and the user's repository is never written.
func (w *Workspace) Compute(ctx context.Context, projects []string) ([]versioning.Result, error) {
	return versioning.Compute(ctx, w.request(projects))
}

// Explicit is the release of project at the explicit version (nerv ship
// --version) in the clone, offline: the engine's last release of the
// project on the branch, and the notes and CHANGELOG section of the
// version from the commits since then.
func (w *Workspace) Explicit(ctx context.Context, project, version string) (versioning.Result, error) {
	req := w.request([]string{project})
	req.Version = version
	results, err := versioning.Compute(ctx, req)
	if err != nil {
		return versioning.Result{}, err
	}
	for _, r := range results {
		if r.Project == project {
			return r, nil
		}
	}
	return versioning.Result{}, fmt.Errorf("the version engine computed no result for project %s", project)
}

// Branch resolves the branch against the configured release branches (its
// prerelease identifier and channel) in the clone.
func (w *Workspace) Branch(ctx context.Context) (versioning.ReleaseBranch, error) {
	return versioning.BranchChannel(ctx, w.request(nil))
}

func (w *Workspace) request(projects []string) versioning.Request {
	return versioning.Request{RepoDir: w.dir, Branch: w.branch, Projects: projects, Config: w.VersionConfig(), Offline: true, Runner: w.versions}
}

// BranchOnOrigin returns origin's current commit of the branch ("" when
// origin no longer has it), read now.
func (w *Workspace) BranchOnOrigin(ctx context.Context) (string, error) {
	fctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	out, err := w.git(fctx, w.dir, w.env, "ls-remote", "--heads", "origin", "refs/heads/"+w.branch)
	if err != nil {
		return "", err
	}
	sha, _, _ := strings.Cut(out, "\t")
	return strings.TrimSpace(sha), nil
}

// writeOriginRewrite appends url.<realURL>.insteadOf = <pushURL> to the git
// config file at path and makes it owner-only: git then reaches origin's real
// URL, with the credentials it holds, while every command line and
// environment only shows pushURL.
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

// pushAddress is the address the clone names origin by: origin's URL without
// credentials (publicURL), or, for a local origin, its resolved path. Other
// addresses cannot be used.
func pushAddress(raw, resolved string) (string, error) {
	if public := publicURL(raw); public != "" {
		return public, nil
	}
	if isLocalRemote(raw) {
		return resolved, nil
	}
	return "", fmt.Errorf("origin's URL (%s) is neither a network address (https, ssh, git) nor a local path: nerv ship cannot read it", redact(raw))
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
