package deployrun

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// fakeFailRunner fails every command, with stderr.
type fakeFailRunner struct{ stderr string }

func (f fakeFailRunner) Run(context.Context, proc.Command) (proc.Result, error) {
	return proc.Result{Stderr: []byte(f.stderr)}, errors.New("exit status 128")
}

// A failed clone's error must not leak the remote URL's credentials, in
// the URL it names or in git's own stderr, which can echo it back.
func TestWorkspaceClone_RedactsCredentialsOnFailure(t *testing.T) {
	url := "https://x-access-token:ghs_abc123@github.com/org/repo.git"
	stderr := "fatal: unable to access '" + url + "/': The requested URL returned error: 403"
	w := workspace{runner: fakeFailRunner{stderr: stderr}, dir: t.TempDir()}
	err := w.clone(context.Background(), url, "main")
	if err == nil {
		t.Fatal("clone: no error")
	}
	if strings.Contains(err.Error(), "ghs_abc123") {
		t.Errorf("clone error leaks the token: %v", err)
	}
	if !strings.Contains(err.Error(), "https://***@github.com/org/repo.git") {
		t.Errorf("clone error %v, want the redacted URL", err)
	}
}

// A remote URL's userinfo (a CI token, commonly https://user:token@host/...)
// must never reach an error, a log line or the deploy report: it is
// dropped, keeping the scheme, host and path. An scp-like ssh address
// (git@host:org/repo) carries no secret and is returned unchanged.
func TestRedactURL_DropsUserinfoKeepsSchemeHostPath(t *testing.T) {
	tests := map[string]string{
		"https://x-access-token:ghs_abc123@github.com/org/repo.git": "https://github.com/org/repo.git",
		"https://user:p@ss@gitlab.example.com/org/repo.git":         "https://gitlab.example.com/org/repo.git",
		"https://ghp_abc123tokenonly@github.com/org/repo.git":       "https://github.com/org/repo.git",
		"https://github.com/org/repo.git":                           "https://github.com/org/repo.git",
		"git@github.com:org/repo.git":                               "git@github.com:org/repo.git",
		"ssh://git@github.com/org/repo.git":                         "ssh://git@github.com/org/repo.git",
		"ssh://git:pass@github.com/org/repo.git":                    "ssh://github.com/org/repo.git",
		"/local/path/repo.git":                                      "/local/path/repo.git",
	}
	for in, want := range tests {
		if got := redactURL(in); got != want {
			t.Errorf("redactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// git itself can echo the failed URL on stderr (e.g. "fatal: unable to
// access '<url>'"); the same userinfo must be masked there too.
func TestRedactUserinfo_MasksCredentialsInFreeText(t *testing.T) {
	in := "fatal: unable to access 'https://x-access-token:ghs_abc123@github.com/org/repo.git/': The requested URL returned error: 403"
	got := redactUserinfo(in)
	if strings.Contains(got, "ghs_abc123") {
		t.Errorf("redactUserinfo(%q) = %q, still leaks the token", in, got)
	}
	if want := "fatal: unable to access 'https://***@github.com/org/repo.git/': The requested URL returned error: 403"; got != want {
		t.Errorf("redactUserinfo = %q, want %q", got, want)
	}
	// scp-like text, and an ssh:// URL's bare username (git's own login
	// name convention, no password), are left alone: no secret to mask.
	if got := redactUserinfo("git@github.com:org/repo.git"); got != "git@github.com:org/repo.git" {
		t.Errorf("redactUserinfo(scp-like) = %q, want unchanged", got)
	}
	if got := redactUserinfo("ssh://git@github.com/org/repo.git"); got != "ssh://git@github.com/org/repo.git" {
		t.Errorf("redactUserinfo(ssh bare username) = %q, want unchanged", got)
	}
}

// The note is JSON.stringify({channels: [channel]}): null for the default
// channel, no HTML escaping.
func TestChannelNote_IsSemanticReleases(t *testing.T) {
	for channel, want := range map[string]string{"": `{"channels":[null]}`, "beta": `{"channels":["beta"]}`, "a<b": `{"channels":["a<b"]}`} {
		if got := channelNote(channel); got != want {
			t.Errorf("channelNote(%q) = %s, want %s", channel, got, want)
		}
	}
}

// The message is compared as git commit -m stores it.
func TestCleanupWhitespace_IsGitsWhitespaceMode(t *testing.T) {
	in := "\n\nchore(release): 1.0.0 [skip ci]  \n\n\n## 1.0.0\t\n\n* a\n\n"
	if got, want := cleanupWhitespace(in), "chore(release): 1.0.0 [skip ci]\n\n## 1.0.0\n\n* a"; got != want {
		t.Errorf("cleanup %q, want %q", got, want)
	}
}
