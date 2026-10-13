package semrel

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fetchArgs returns the arguments of a git fetch after `-C dir fetch`.
func fetchArgs(c Command) []string {
	args := c.Args
	for len(args) >= 2 && args[0] == "-C" {
		args = args[2:]
	}
	return args[1:]
}

// envValue returns the last value of name in env.
func envValue(env []string, name string) (string, bool) {
	value, found := "", false
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			value, found = v, true
		}
	}
	return value, found
}

// fetchURL returns the URL a git fetch command reads from: the remote
// argument itself, or the url configured for that remote name through
// GIT_CONFIG_KEY_<n>/GIT_CONFIG_VALUE_<n>.
func fetchURL(c Command) string {
	remote := ""
	for _, a := range fetchArgs(c) {
		if !strings.HasPrefix(a, "-") {
			remote = a
			break
		}
	}
	for _, kv := range c.Env {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "GIT_CONFIG_KEY_") && v == "remote."+remote+".url" {
			if url, ok := envValue(c.Env, "GIT_CONFIG_VALUE_"+strings.TrimPrefix(k, "GIT_CONFIG_KEY_")); ok {
				return url
			}
		}
	}
	return remote
}

// originRepo is fakeRepo with origin's URL replaced by url and every other
// command answered by fakeRepo; the runner's plan answers ok.
func originRepo(t *testing.T, root, url string) *fakeRunner {
	t.Helper()
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return okPlan()
		}
		return respond(map[string]any{"ok": true})
	})
	inner := f.handle
	f.handle = func(c Command) (Result, error) {
		if isGit(c, "remote", "get-url", "origin") {
			return Result{Stdout: []byte(url + "\n")}, nil
		}
		return inner(c)
	}
	return f
}

func onlyFetch(t *testing.T, f *fakeRunner) Command {
	t.Helper()
	fetches := f.find(func(c Command) bool { return isGit(c, "fetch") })
	if len(fetches) != 1 {
		t.Fatalf("fetches %d, want 1", len(fetches))
	}
	return fetches[0]
}

func TestCompute_OriginFetchFailureHidesCredentials(t *testing.T) {
	const url = "https://nerv:s3cr3t-pass@git.example.com/team/repo.git?access_token=tok-123"
	root := t.TempDir()
	f := originRepo(t, root, url)
	inner := f.handle
	f.handle = func(c Command) (Result, error) {
		if isGit(c, "fetch") {
			// git echoes the URL it could not read.
			return Result{Stderr: []byte("fatal: unable to access '" + url + "/': The requested URL returned error: 403")}, errors.New("exit status 128")
		}
		return inner(c)
	}
	_, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir(), Env: []string{"PATH=/bin"}})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, secret := range []string{"s3cr3t-pass", "tok-123", "nerv:"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error leaks %q: %v", secret, err)
		}
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "git.example.com") {
		t.Errorf("error lost the useful part: %v", err)
	}
	for _, c := range f.calls {
		for _, a := range c.Args {
			if strings.Contains(a, "s3cr3t-pass") {
				t.Errorf("credentials on the command line (visible to every local user): %v", c)
			}
		}
	}
	if got := fetchURL(onlyFetch(t, f)); got != url {
		t.Errorf("fetch reads %q, want the origin URL", got)
	}
}

func TestCompute_FetchNeverPrompts(t *testing.T) {
	tests := []struct {
		name       string
		env        []string
		configured string // core.sshCommand of the repository
		want       string // GIT_SSH_COMMAND of the fetch; "-" means not set
	}{
		{name: "plain ssh", want: "ssh -o BatchMode=yes"},
		{name: "ssh command from the environment", env: []string{"GIT_SSH_COMMAND=ssh -i /keys/id"}, want: "ssh -i /keys/id -o BatchMode=yes"},
		{name: "ssh command from core.sshCommand", configured: "ssh -F /cfg", want: "ssh -F /cfg -o BatchMode=yes"},
		{name: "windows ssh path", env: []string{`GIT_SSH_COMMAND=C:\Windows\System32\OpenSSH\ssh.exe`}, want: `C:\Windows\System32\OpenSSH\ssh.exe -o BatchMode=yes`},
		{name: "a non-ssh command is left alone", env: []string{"GIT_SSH_COMMAND=plink -batch"}, want: "plink -batch"},
		{name: "GIT_SSH program is left alone", env: []string{"GIT_SSH=/usr/bin/plink"}, want: "-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			f := originRepo(t, root, "git@github.com:example/repo.git")
			inner := f.handle
			f.handle = func(c Command) (Result, error) {
				if isGit(c, "config", "--get", "core.sshCommand") {
					if tt.configured == "" {
						return Result{}, errors.New("exit status 1")
					}
					return Result{Stdout: []byte(tt.configured + "\n")}, nil
				}
				return inner(c)
			}
			env := append([]string{"PATH=/bin"}, tt.env...)
			if _, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir(), Env: env}); err != nil {
				t.Fatal(err)
			}
			fetch := onlyFetch(t, f)
			got, ok := envValue(fetch.Env, "GIT_SSH_COMMAND")
			if !ok {
				got = "-"
			}
			if got != tt.want {
				t.Errorf("GIT_SSH_COMMAND = %q, want %q", got, tt.want)
			}
			for _, kv := range []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"} {
				if !slices.Contains(fetch.Env, kv) {
					t.Errorf("fetch env lacks %s", kv)
				}
			}
			if fetch.Stdin != nil {
				t.Errorf("fetch has a stdin; want none (detached)")
			}
		})
	}
}

// deadlineRunner records the deadline of the fetch's context and answers it
// with fetch.
type deadlineRunner struct {
	*fakeRunner
	fetch    func(ctx context.Context) (Result, error)
	deadline time.Time
}

func (d *deadlineRunner) Run(ctx context.Context, c Command) (Result, error) {
	if isGit(c, "fetch") {
		d.deadline, _ = ctx.Deadline()
		return d.fetch(ctx)
	}
	return d.fakeRunner.Run(ctx, c)
}

func TestCompute_FetchIsBounded(t *testing.T) {
	root := t.TempDir()
	r := &deadlineRunner{fakeRunner: originRepo(t, root, "https://github.com/example/repo.git"), fetch: func(context.Context) (Result, error) { return Result{}, nil }}
	start := time.Now()
	if _, err := (&Engine{Runner: r}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if r.deadline.IsZero() {
		t.Fatal("the origin fetch has no deadline")
	}
	if d := r.deadline.Sub(start); d < 55*time.Second || d > 65*time.Second {
		t.Errorf("default fetch deadline %s, want about 60s", d)
	}
}

func TestCompute_RelativeOriginResolvedAgainstTheRepository(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   func(root string) string
	}{
		{"relative path", "../origin.git", func(root string) string { return filepath.Join(filepath.Dir(root), "origin.git") }},
		{"dot path", "./sub/origin", func(root string) string { return filepath.Join(root, "sub", "origin") }},
		{"bare relative name", "origin-dir", func(root string) string { return filepath.Join(root, "origin-dir") }},
		{"absolute path", "/srv/git/repo.git", func(string) string { return "/srv/git/repo.git" }},
		{"https URL", "https://github.com/example/repo.git", func(string) string { return "https://github.com/example/repo.git" }},
		{"file URL", "file:///srv/git/repo.git", func(string) string { return "file:///srv/git/repo.git" }},
		{"scp-like ssh", "git@github.com:example/repo.git", func(string) string { return "git@github.com:example/repo.git" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			f := originRepo(t, root, tt.origin)
			if _, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()}); err != nil {
				t.Fatal(err)
			}
			if got, want := fetchURL(onlyFetch(t, f)), tt.want(root); got != want {
				t.Errorf("fetch reads %q, want %q", got, want)
			}
		})
	}
}

func TestCompute_FetchTimeoutIsAnError(t *testing.T) {
	root := t.TempDir()
	r := &deadlineRunner{fakeRunner: originRepo(t, root, "https://github.com/example/repo.git"), fetch: func(ctx context.Context) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}}
	start := time.Now()
	_, err := (&Engine{Runner: r}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir(), FetchTimeout: 50 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "timed out after 50ms") || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("err = %v", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("Compute took %s with a 50ms fetch timeout", took)
	}
}

func TestRedact(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://user:pass@github.com/a/b.git", "https://***@github.com/a/b.git"},
		{"fatal: unable to access 'https://x-access-token:ghs_abc@github.com/a/b.git/': 403", "fatal: unable to access 'https://***@github.com/a/b.git/': 403"},
		{"ssh://git@host:22/repo", "ssh://***@host:22/repo"},
		{"https://h/r.git?access_token=abc&ref=main", "https://h/r.git?access_token=***&ref=main"},
		{"https://h/r?private_token=abc", "https://h/r?private_token=***"},
		{"https://github.com/a/b.git", "https://github.com/a/b.git"},
		{"git@github.com:a/b.git", "git@github.com:a/b.git"},
		{"/srv/git/repo.git", "/srv/git/repo.git"},
	}
	for _, tt := range tests {
		if got := redact(tt.in); got != tt.want {
			t.Errorf("redact(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
