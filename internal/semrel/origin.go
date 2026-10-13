package semrel

import (
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DefaultFetchTimeout bounds the read of release tags and notes from origin
// when Options.FetchTimeout is zero.
const DefaultFetchTimeout = 60 * time.Second

// originRemote is the remote name the mirror fetches origin's tags and notes
// from. Its URL is configured through the environment (GIT_CONFIG_COUNT), never
// on the command line: argv is visible to every local user and ends up in
// error messages, and the URL may carry credentials.
const originRemote = "nerv-origin"

// origin is what the mirror fetches from the user's origin remote.
type origin struct {
	// URL is origin's URL, a local path already made absolute; empty skips
	// the fetch.
	URL string
	// Public is publicURL of origin's URL: the repository address the
	// release notes link to; empty when there is none.
	Public string
	// SSHCommand is the repository's core.sshCommand, if any.
	SSHCommand string
	Timeout    time.Duration
}

var (
	urlSchemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)
	// scpLikeRe is git's scp-like ssh syntax, [user@]host:path: a colon
	// before any slash.
	scpLikeRe = regexp.MustCompile(`^[^/\\:]+:`)
	driveRe   = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
)

// resolveRemoteURL makes a local-path remote URL absolute, resolved against
// the repository's top-level directory as git resolves it (the mirror lives in
// a temporary directory, where a relative path means something else). URLs
// with a scheme and scp-like ssh addresses are returned unchanged.
func resolveRemoteURL(url, top string) string {
	switch {
	case url == "", urlSchemeRe.MatchString(url), driveRe.MatchString(url), filepath.IsAbs(url):
		return url
	case scpLikeRe.MatchString(url):
		return url
	}
	return filepath.Join(top, url)
}

// sshBatchOption makes ssh fail instead of asking for a password or a
// passphrase on the terminal (ssh opens /dev/tty itself, so a detached stdin
// is not enough).
const sshBatchOption = "-o BatchMode=yes"

// fetchSSHCommand returns the GIT_SSH_COMMAND for the fetch, following git's
// precedence (GIT_SSH_COMMAND, then core.sshCommand, then GIT_SSH, then ssh):
// an ssh command gets BatchMode appended; a command that is not ssh (plink,
// a wrapper script) cannot safely take ssh options and is kept as it is
// (ok false: leave the environment alone), as is a GIT_SSH program, which
// git runs with its own argument convention.
func fetchSSHCommand(env []string, configured string) (string, bool) {
	cmd, ok := lookupEnv(env, "GIT_SSH_COMMAND")
	if !ok || strings.TrimSpace(cmd) == "" {
		cmd = strings.TrimSpace(configured)
	}
	if cmd == "" {
		if gitSSH, ok := lookupEnv(env, "GIT_SSH"); ok && gitSSH != "" {
			return "", false
		}
		return "ssh " + sshBatchOption, true
	}
	if !isSSH(cmd) {
		return "", false
	}
	return cmd + " " + sshBatchOption, true
}

// isSSH reports whether a shell command line runs OpenSSH's ssh client.
func isSSH(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	var first string
	if q := cmd[0]; q == '"' || q == '\'' {
		// A quoted program path may contain spaces ("C:\Program Files\...").
		first, _, _ = strings.Cut(cmd[1:], string(q))
	} else {
		first = strings.Fields(cmd)[0]
	}
	if i := strings.LastIndexAny(first, `/\`); i >= 0 {
		first = first[i+1:]
	}
	first = strings.ToLower(first)
	return first == "ssh" || first == "ssh.exe"
}

// fetchEnv is env for the origin fetch: origin's URL as the nerv-origin
// remote (GIT_CONFIG_COUNT, extending any count already set), no credential
// or ssh prompt, and no Git Credential Manager dialog.
func fetchEnv(env []string, o origin) []string {
	out := slices.Clone(env)
	n := 0
	if v, ok := lookupEnv(env, "GIT_CONFIG_COUNT"); ok {
		n, _ = strconv.Atoi(v)
	}
	idx := strconv.Itoa(n)
	out = setEnv(out, "GIT_CONFIG_KEY_"+idx, "remote."+originRemote+".url")
	out = setEnv(out, "GIT_CONFIG_VALUE_"+idx, o.URL)
	out = setEnv(out, "GIT_CONFIG_COUNT", strconv.Itoa(n+1))
	out = setEnv(out, "GIT_TERMINAL_PROMPT", "0")
	out = setEnv(out, "GCM_INTERACTIVE", "never")
	if ssh, ok := fetchSSHCommand(env, o.SSHCommand); ok {
		out = setEnv(out, "GIT_SSH_COMMAND", ssh)
	}
	return out
}

// lookupEnv returns the last value of name in env.
func lookupEnv(env []string, name string) (string, bool) {
	value, found := "", false
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			value, found = v, true
		}
	}
	return value, found
}

// setEnv returns env with every name entry replaced by name=value.
func setEnv(env []string, name, value string) []string {
	out := slices.DeleteFunc(env, func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		return k == name
	})
	return append(out, name+"="+value)
}

// publicSchemes are the URL schemes of a remote other people can reach.
var publicSchemes = []string{"http", "https", "ssh", "git", "git+ssh", "ssh+git"}

// publicURL is the address of the user's origin to name in release-notes
// links, as semantic-release would read it from remote.origin.url in CI:
// scheme URLs lose their user info, query and fragment (credentials never
// reach the notes or a command line), scp-like ssh addresses stay as they
// are, and a local repository (a path, a file: URL) or no origin gives "",
// as there is no public page to link to.
func publicURL(raw string) string {
	switch {
	case raw == "", driveRe.MatchString(raw), filepath.IsAbs(raw):
		return ""
	case urlSchemeRe.MatchString(raw):
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || !slices.Contains(publicSchemes, strings.ToLower(u.Scheme)) {
			return ""
		}
		u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
		return u.String()
	case scpLikeRe.MatchString(raw):
		return raw
	}
	return ""
}
