package deployrun

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployconfig"
)

// fakeTool records its name, argv and stdin, and answers as the real tool
// would for what deploy runs: gh release view/create keep a release per
// tag in the fake's directory; docker push fails while fail-push exists;
// a nerv provider operation answers failed while fail-<operation> exists;
// the fake nerv runs the real release-ssh-compose preflight and every real
// release-script operation (the test binary, see TestMain), and prints an ok
// response for any other operation.
const fakeTool = `#!/bin/sh
name=$(basename "$0")
{ printf '%s' "$name"; for a in "$@"; do printf ' [%s]' "$a"; done; printf '\n'; } >> "$FAKE_DIR/log"
cat > "$FAKE_DIR/$name.stdin"
case "$name" in
gh)
  if [ "$2" = view ]; then
    [ -f "$FAKE_DIR/release-$3" ] && exit 0
    echo "release not found" >&2; exit 1
  fi
  [ "$2" = create ] && touch "$FAKE_DIR/release-$3"
  ;;
docker)
  if [ "$1" = push ] && [ -f "$FAKE_DIR/fail-push" ]; then echo "denied: push access" >&2; exit 1; fi
  ;;
nerv)
  if [ "$1" = provider ] && { [ "$3" = preflight ] || [ "$2" = release-script ]; }; then
    NERV_FAKE_PROVIDER=1 exec "$NERV_TEST_BINARY" "$@" < "$FAKE_DIR/nerv.stdin"
  fi
  if [ -f "$FAKE_DIR/fail-$3" ]; then echo '{"status":"failed","message":"'"$3"' broke"}'; exit 0; fi
  echo '{"status":"ok","message":"'"$3"' done"}'
  ;;
esac
exit 0
`

// world is one test's world: a bare origin, the user's clone of it (with
// files, when given, in its first commit), an artifact of the clone's
// develop head, and fake gh, docker, ssh and nerv on PATH.
type world struct {
	t                  *testing.T
	origin, repo, fake string
	base               string
	dir                string
	cfg                *deployconfig.Deploy
}

const (
	notes     = "## 1.0.0-alpha.1 (2026-10-23)\n\n### Features\n\n* first ([abc](x)) $(touch /tmp/pwned)\n"
	changelog = "# Changelog\n\n## 1.0.0-alpha.1 (2026-10-23)\n\n* first\n"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitOK(dir string, args ...string) bool {
	return exec.Command("git", append([]string{"-C", dir}, args...)...).Run() == nil
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func newEnv(t *testing.T, files ...map[string]string) *world {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	gitconfig := filepath.Join(root, "gitconfig")
	writeFile(t, gitconfig, "[user]\n\tname = Test\n\temail = test@example.com\n[commit]\n\tgpgsign = false\n[tag]\n\tgpgsign = false\n[init]\n\tdefaultBranch = main\n", 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	e := &world{t: t, origin: filepath.Join(root, "origin.git"), repo: filepath.Join(root, "repo"), fake: filepath.Join(root, "fake")}
	git(t, root, "init", "-q", "--bare", e.origin)
	git(t, root, "clone", "-q", e.origin, e.repo)
	writeFile(t, filepath.Join(e.repo, "app.txt"), "one\n", 0o644)
	writeFile(t, filepath.Join(e.repo, "CHANGELOG.md"), "# Changelog\n", 0o644)
	for _, set := range files {
		for name, content := range set {
			writeFile(t, filepath.Join(e.repo, filepath.FromSlash(name)), content, 0o644)
		}
	}
	git(t, e.repo, "add", ".")
	git(t, e.repo, "commit", "-q", "-m", "feat: first")
	git(t, e.repo, "push", "-q", "origin", "HEAD:refs/heads/main", "HEAD:refs/heads/develop")
	git(t, e.repo, "checkout", "-q", "-b", "develop", "origin/develop")
	e.base = git(t, e.repo, "rev-parse", "HEAD")

	bin := filepath.Join(e.fake, "bin")
	for _, tool := range []string{"gh", "docker", "ssh", "sshpass", "nerv"} {
		writeFile(t, filepath.Join(bin, tool), fakeTool, 0o755)
	}
	t.Setenv("FAKE_DIR", e.fake)
	exe, exeErr := os.Executable()
	if exeErr != nil {
		t.Fatal(exeErr)
	}
	t.Setenv("NERV_TEST_BINARY", exe)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	e.dockerConfig(`{"auths":{"reg.example":{"auth":"dXNlcjpwYXNz"}}}`)

	e.writeArtifact([]string{"reg.example/app:1.0.0-alpha.1"}, "app.tar")
	e.cfg = &deployconfig.Deploy{
		Host:    "github",
		Targets: []deployconfig.Target{{Name: "staging", Provider: "ssh-compose", Config: map[string]string{"environment": "staging"}}},
		Git:     deployconfig.Git{CommitMessage: deployconfig.DefaultCommitMessage, Remote: "origin"},
	}
	return e
}

// writeArtifact (re)writes the artifact of the clone's develop head with
// images and one image tar per name in tars.
func (e *world) writeArtifact(images []string, tars ...string) {
	t := e.t
	t.Helper()
	var outputs []artifact.OutputSource
	for _, name := range tars {
		src := filepath.Join(t.TempDir(), name)
		writeFile(t, src, "image bytes of "+name, 0o644)
		outputs = append(outputs, artifact.OutputSource{Path: name, Kind: artifact.KindImageTar, From: src})
	}
	dir, err := artifact.Write(e.repo, artifact.Input{
		Manifest: artifact.Manifest{
			Project: "repo", Version: "1.0.0-alpha.1", Channel: "alpha", Tag: "v1.0.0-alpha.1",
			BaseCommit: e.base, Branch: "develop", BuildProvider: "docker-compose", Images: images,
		},
		Notes: notes, ChangelogSection: "## 1.0.0-alpha.1 (2026-10-23)\n\n* first\n",
		Outputs:      outputs,
		ReleaseFiles: []artifact.ReleaseFileSource{{Path: "CHANGELOG.md", Content: []byte(changelog)}},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	e.dir = dir
}

// dockerConfig points DOCKER_CONFIG at a directory whose config.json is
// config.
func (e *world) dockerConfig(config string) {
	e.t.Helper()
	dir := filepath.Join(e.t.TempDir(), "docker")
	writeFile(e.t, filepath.Join(dir, "config.json"), config, 0o600)
	e.t.Setenv("DOCKER_CONFIG", dir)
}

func (e *world) options() Options {
	return Options{
		RepoRoot: e.repo, ArtifactDir: e.dir, Config: e.cfg, Providers: nerv.ExtensionsFS(),
		Nerv: filepath.Join(e.fake, "bin", "nerv"), Output: io.Discard,
	}
}

func (e *world) run(opts Options) (Report, error) {
	e.t.Helper()
	return Run(context.Background(), opts)
}

// log returns the fake tools' calls, one per line.
func (e *world) log() []string {
	data, _ := os.ReadFile(filepath.Join(e.fake, "log"))
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// preflightCall is the fake log line of the ssh-compose target's
// preflight, which checks its config before anything is written.
const preflightCall = "nerv [provider] [release-ssh-compose] [preflight]"

// effects returns the fake tools' calls that act (every call but the
// preflights); none is [""].
func (e *world) effects() []string {
	out := []string{}
	for _, c := range e.log() {
		if c != preflightCall && c != "" {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// originRefs lists every ref of origin with its object.
func (e *world) originRefs() string {
	return git(e.t, e.origin, "for-each-ref", "--format=%(refname) %(objectname)")
}

// pushOther pushes a commit writing file on origin's develop, from another
// clone.
func (e *world) pushOther(file, content string) {
	e.t.Helper()
	other := filepath.Join(e.t.TempDir(), "other")
	git(e.t, filepath.Dir(other), "clone", "-q", "--branch", "develop", e.origin, other)
	writeFile(e.t, filepath.Join(other, file), content, 0o644)
	git(e.t, other, "add", ".")
	git(e.t, other, "commit", "-q", "-m", "fix: other work")
	git(e.t, other, "push", "-q", "origin", "develop")
}

func statuses(r Report) map[string]string {
	m := map[string]string{}
	for _, s := range r.Steps {
		m[s.Name] = s.Status
	}
	return m
}

func wantStatuses(t *testing.T, r Report, want map[string]string) {
	t.Helper()
	got := statuses(r)
	for name, status := range want {
		if got[name] != status {
			t.Errorf("step %s: %q, want %q (all: %v)", name, got[name], status, got)
		}
	}
}
