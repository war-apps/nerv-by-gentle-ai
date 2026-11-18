package deployrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// NotesRefPrefix is semantic-release's channel note ref (lib/definitions/
// constants.js GIT_NOTE_REF): refs/notes/semantic-release-<tag>.
const NotesRefPrefix = "refs/notes/semantic-release-"

// workspace is the throwaway clone of origin deploy works in.
type workspace struct {
	runner proc.Runner
	dir    string
	env    []string
}

// git runs git in the workspace and returns its trimmed stdout.
func (w workspace) git(ctx context.Context, args ...string) (string, error) {
	res, err := w.runner.Run(ctx, proc.Command{Name: "git", Args: append([]string{"-C", w.dir}, args...), Env: w.env})
	if err != nil {
		detail := strings.TrimSpace(string(res.Stderr))
		if detail == "" {
			detail = strings.TrimSpace(string(res.Stdout))
		}
		if detail != "" {
			return "", fmt.Errorf("git %s: %w: %s", subcommand(args), err, detail)
		}
		return "", fmt.Errorf("git %s: %w", subcommand(args), err)
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// subcommand is the git subcommand of args, after its -c <key=value>
// options ("push" for -c credential.interactive=never push ...).
func subcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" {
			i++
			continue
		}
		return args[i]
	}
	return "git"
}

// ok reports whether git succeeds (a yes/no query).
func (w workspace) ok(ctx context.Context, args ...string) bool {
	_, err := w.git(ctx, args...)
	return err == nil
}

// resolve returns the commit rev names, "" when it does not exist.
func (w workspace) resolve(ctx context.Context, rev string) string {
	out, err := w.git(ctx, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return ""
	}
	return out
}

// clone clones remoteURL's branch into the workspace with origin's tags
// and semantic-release notes. Credentials stay the user's git setup;
// prompts are refused. remoteURL's userinfo (a CI token, commonly
// https://user:token@host/...) must never reach an error, a log line or
// the deploy report, so it, and any the same credentials git's own
// stderr echoes back, are redacted before they appear in the error.
func (w workspace) clone(ctx context.Context, remoteURL, branch string) error {
	res, err := w.runner.Run(ctx, proc.Command{Name: "git", Env: w.env,
		Args: []string{"-c", "credential.interactive=never", "clone", "--quiet", "--no-checkout", "--branch", branch, "--", remoteURL, w.dir}})
	if err != nil {
		detail := redactUserinfo(strings.TrimSpace(string(res.Stderr)))
		return fmt.Errorf("git clone %s (branch %s): %w: %s", redactURL(remoteURL), branch, err, detail)
	}
	_, err = w.git(ctx, "-c", "credential.interactive=never", "fetch", "--quiet", "--tags", "origin",
		"+refs/notes/semantic-release*:refs/notes/semantic-release*")
	return err
}

// hasSchemeRe matches a URL scheme ("https://", "ssh://", ...); an scp-like
// ssh address (git@host:org/repo) has none and carries no secret.
var hasSchemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// schemeUserinfoRe matches a URL's scheme and userinfo in free text
// ("scheme://userinfo@"), to find it wherever git's own stderr echoes a
// failed URL back.
var schemeUserinfoRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*)://([^/\s@'"]+)@`)

// isSecretUserinfo reports whether a URL's userinfo may hold a secret:
// any userinfo on a scheme other than ssh (https://<token>@host is a
// common CI form with no password part at all), or a password part on
// any scheme. A bare ssh username (ssh://git@host, git@host:org/repo) is
// git's own convention for the login name, not a secret.
func isSecretUserinfo(scheme, userinfo string) bool {
	switch strings.ToLower(scheme) {
	case "ssh", "git+ssh", "ssh+git", "git":
		return strings.Contains(userinfo, ":")
	default:
		return true
	}
}

// redactURL drops raw's userinfo when it may hold a secret (isSecretUserinfo),
// keeping the scheme, host and path (https://user:token@host/path ->
// https://host/path): a cloned repository's CI remote commonly carries a
// token there, and it must never reach an error, a log line or the
// deploy report. An scp-like ssh address, anything without a URL scheme,
// or a bare ssh username, is returned unchanged.
func redactURL(raw string) string {
	if !hasSchemeRe.MatchString(raw) {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil || !isSecretUserinfo(u.Scheme, u.User.String()) {
		return raw
	}
	u.User = nil
	return u.String()
}

// redactUserinfo masks every secret URL userinfo (isSecretUserinfo) found
// in free text (git's own stderr can echo the URL it failed to read).
func redactUserinfo(s string) string {
	return schemeUserinfoRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := schemeUserinfoRe.FindStringSubmatch(m)
		scheme, userinfo := sub[1], sub[2]
		if !isSecretUserinfo(scheme, userinfo) {
			return m
		}
		return scheme + "://***@"
	})
}

// releaseState is what origin already holds of this release.
type releaseState struct {
	// commit is the release commit to tag ("" until made).
	commit string
	// commitOnOrigin: the release commit is origin's branch head or an
	// ancestor of it.
	commitOnOrigin bool
	tagOnOrigin    bool
	notesOnOrigin  bool
}

// matches reports whether commit holds the release: every release file
// with the artifact's content, and the expected message (or, when the
// release changes no file, the base commit itself).
func (w workspace) matches(ctx context.Context, commit, base, message string, files []artifact.ReleaseFile) bool {
	if !w.hasFiles(ctx, commit, files) {
		return false
	}
	if commit == base {
		return true
	}
	got, err := w.git(ctx, "log", "-1", "--format=%B", commit)
	return err == nil && got == cleanupWhitespace(message)
}

// cleanupWhitespace is git commit's --cleanup=whitespace (the mode of
// git commit -m, as @semantic-release/git commits): trailing whitespace
// removed from each line, runs of blank lines collapsed, and leading and
// trailing blank lines removed.
func cleanupWhitespace(message string) string {
	var out []string
	blank := false
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimRight(line, " \t\r\v\f")
		if line == "" {
			blank = len(out) > 0
			continue
		}
		if blank {
			out = append(out, "")
			blank = false
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// hasFiles reports whether every release file has its release content at
// commit.
func (w workspace) hasFiles(ctx context.Context, commit string, files []artifact.ReleaseFile) bool {
	for _, f := range files {
		res, err := w.runner.Run(ctx, proc.Command{Name: "git", Args: []string{"-C", w.dir, "cat-file", "blob", commit + ":" + f.Path}, Env: w.env})
		if err != nil {
			return false
		}
		h := sha256.Sum256(res.Stdout)
		if hex.EncodeToString(h[:]) != f.SHA256 {
			return false
		}
	}
	return true
}

// commitRelease checks out at, writes the release files from the artifact
// and commits them with message. When they change nothing, at itself is
// the release commit.
func (w workspace) commitRelease(ctx context.Context, at, artifactDir, message string, files []artifact.ReleaseFile) (string, bool, error) {
	if _, err := w.git(ctx, "checkout", "--quiet", "--force", "--detach", at); err != nil {
		return "", false, err
	}
	for _, f := range files {
		if err := w.checkReleasePath(f.Path); err != nil {
			return "", false, err
		}
		data, err := os.ReadFile(filepath.Join(artifactDir, filepath.FromSlash(f.Stored)))
		if err != nil {
			return "", false, err
		}
		dst := filepath.Join(w.dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", false, err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return "", false, err
		}
		if _, err := w.git(ctx, "add", "--", f.Path); err != nil {
			return "", false, err
		}
	}
	if w.ok(ctx, "diff", "--cached", "--quiet") {
		return at, false, nil
	}
	msgFile := filepath.Join(filepath.Dir(w.dir), "commit-message")
	if err := os.WriteFile(msgFile, []byte(message), 0o600); err != nil {
		return "", false, err
	}
	if _, err := w.git(ctx, "commit", "--quiet", "--no-verify", "--cleanup=whitespace", "--file", msgFile); err != nil {
		return "", false, err
	}
	commit, err := w.git(ctx, "rev-parse", "HEAD")
	return commit, true, err
}

// checkReleasePath refuses a release file that would land in the clone's
// git directory, or be written through a symlink the repository holds.
func (w workspace) checkReleasePath(p string) error {
	if !artifact.SafeReleasePath(p) {
		return &refusal.Error{Err: fmt.Errorf("release file %q is not a repository path outside .git", p)}
	}
	dir := w.dir
	for _, part := range strings.Split(p, "/") {
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return &refusal.Error{Err: fmt.Errorf("release file %q: cannot check %s for a symlink: %v", p, part, err)}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &refusal.Error{Err: fmt.Errorf("release file %q is written through the symlink %s in the repository", p, part)}
		}
	}
	return nil
}

// errConflict is a release commit that does not apply on origin's new head.
var errConflict = errors.New("conflict")

// rebase re-applies commit (made on the base) on head; a conflict aborts
// and is errConflict.
func (w workspace) rebase(ctx context.Context, commit, head string) (string, error) {
	if _, err := w.git(ctx, "checkout", "--quiet", "--force", "--detach", head); err != nil {
		return "", err
	}
	if _, err := w.git(ctx, "cherry-pick", "--allow-empty", commit); err != nil {
		w.git(ctx, "cherry-pick", "--abort")
		return "", fmt.Errorf("%w: %v", errConflict, err)
	}
	return w.git(ctx, "rev-parse", "HEAD")
}

// notesRef is the channel note ref of tag.
func notesRef(tag string) string { return NotesRefPrefix + tag }

// channelNote is the note semantic-release writes (JSON.stringify of
// {channels: [channel]}, null for the default channel).
func channelNote(channel string) string {
	var ch any
	if channel != "" {
		ch = channel
	}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string][]any{"channels": {ch}})
	return strings.TrimSpace(b.String())
}
