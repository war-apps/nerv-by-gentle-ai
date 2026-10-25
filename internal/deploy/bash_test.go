package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
)

// bashRepo is a git repository holding scripts, tagged v1.2.0, the way the
// deploy clone holds the release commit.
type bashRepo struct {
	t        *testing.T
	dir, out string
}

func newBashRepo(t *testing.T, files map[string]string) *bashRepo {
	t.Helper()
	for _, tool := range []string{"git", "bash"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not available")
		}
	}
	root := t.TempDir()
	gitconfig := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[user]\n\tname = T\n\temail = t@example.com\n[commit]\n\tgpgsign = false\n[tag]\n\tgpgsign = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	r := &bashRepo{t: t, dir: filepath.Join(root, "repo"), out: filepath.Join(root, "out")}
	if err := os.MkdirAll(r.out, 0o755); err != nil {
		t.Fatal(err)
	}
	r.git("init", "-q", r.dir)
	for name, content := range files {
		r.write(name, content)
	}
	r.git("-C", r.dir, "add", "-A")
	r.git("-C", r.dir, "commit", "-q", "-m", "release")
	r.git("-C", r.dir, "tag", "v1.2.0")
	return r
}

func (r *bashRepo) git(args ...string) {
	r.t.Helper()
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *bashRepo) write(name, content string) {
	r.t.Helper()
	path := filepath.Join(r.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// request is the request nerv deploy sends a deploy-bash operation.
func bashRequest(op string, config map[string]any) []byte {
	req := extension.Request{
		Contract: extension.ContractVersion, Extension: "deploy", Slot: "target", Provider: "bash", Operation: op, Config: config,
		Payload: map[string]any{
			"target": "staging", "artifact_dir": "/art",
			"artifact": map[string]any{
				"project": "api", "version": "1.2.0", "channel": "", "tag": "v1.2.0", "base_commit": "abc", "branch": "main",
				"build_provider": "bash", "images": []string{"r/api:1.2.0", "r/web:1.2.0"},
			},
		},
	}
	data, _ := json.Marshal(req)
	return data
}

// run runs op in dir and returns its exit code and response.
func (r *bashRepo) run(op, dir string, config map[string]any) (int, extension.Response, string) {
	r.t.Helper()
	var stdout, stderr bytes.Buffer
	code := RunBashOp(context.Background(), op, bytes.NewReader(bashRequest(op, config)), &stdout, BashDeps{
		Dir: dir, Env: append(os.Environ(), "OUT="+r.out), Output: &stderr, TempDir: r.t.TempDir(),
	})
	resp, ok := extension.ParseResponse(stdout.Bytes())
	if !ok {
		r.t.Fatalf("%s printed no response: %q (stderr %q)", op, stdout.String(), stderr.String())
	}
	return code, resp, stderr.String()
}

func (r *bashRepo) read(name string) string {
	data, _ := os.ReadFile(filepath.Join(r.out, name))
	return string(data)
}

const recordScript = `cat > "$OUT/stdin"
env | grep '^NERV_DEPLOY_' | sort > "$OUT/env"
pwd > "$OUT/pwd"
cat lib/data.txt > "$OUT/data"
echo "deploying" >&2
echo '{"status":"ok","message":"deployed api","data":{"url":"https://staging"}}'
`

// deploy runs the release commit's script with bash, in that commit's
// tree: the request on its stdin, the deploy variables in its
// environment, and its response is the operation's.
func TestRunBashOp_DeployRunsTheReleasedScript(t *testing.T) {
	r := newBashRepo(t, map[string]string{"deploy/run.sh": recordScript, "lib/data.txt": "released\n"})
	// The working tree moved on: the release commit's script runs.
	r.write("lib/data.txt", "unreleased\n")
	code, resp, stderr := r.run("deploy", r.dir, map[string]any{"script": "deploy/run.sh"})
	if code != 0 || resp.Status != extension.StatusOK || resp.Message != "deployed api" || string(resp.Data) != `{"url":"https://staging"}` {
		t.Fatalf("deploy = %d %+v (stderr %q)", code, resp, stderr)
	}
	if !strings.Contains(stderr, "deploying") {
		t.Errorf("the script's stderr is not streamed: %q", stderr)
	}
	if got := r.read("data"); got != "released\n" {
		t.Errorf("the script ran in %q, not the release commit's tree (data %q)", r.read("pwd"), got)
	}
	if strings.HasPrefix(strings.TrimSpace(r.read("pwd")), r.dir) {
		t.Errorf("the script ran in the repository %s, not a tree of the release commit", r.read("pwd"))
	}
	var req extension.Request
	if err := json.Unmarshal([]byte(r.read("stdin")), &req); err != nil || req.Operation != "deploy" || req.Provider != "bash" || req.Config["script"] != "deploy/run.sh" {
		t.Errorf("script stdin %q: %v", r.read("stdin"), err)
	}
	for _, v := range []string{
		"NERV_DEPLOY_ARTIFACT_DIR=/art", "NERV_DEPLOY_BASE_COMMIT=abc", "NERV_DEPLOY_BRANCH=main", "NERV_DEPLOY_BUILD_PROVIDER=bash",
		"NERV_DEPLOY_CHANGELOG_FILE=/art/CHANGELOG.section.md", "NERV_DEPLOY_CHANNEL=", "NERV_DEPLOY_IMAGES=r/api:1.2.0 r/web:1.2.0",
		"NERV_DEPLOY_NOTES_FILE=/art/notes.md", "NERV_DEPLOY_OPERATION=deploy", "NERV_DEPLOY_PRERELEASE=false", "NERV_DEPLOY_PROJECT=api",
		"NERV_DEPLOY_TAG=v1.2.0", "NERV_DEPLOY_TARGET=staging", "NERV_DEPLOY_VERSION=1.2.0",
	} {
		if !strings.Contains(r.read("env"), v+"\n") {
			t.Errorf("script environment lacks %s:\n%s", v, r.read("env"))
		}
	}
	if !strings.Contains(r.read("env"), "NERV_DEPLOY_COMMIT=") {
		t.Errorf("script environment lacks the release commit:\n%s", r.read("env"))
	}
}

// A script answering failed, blocked or needs_input fails the operation
// with its message; so does a non-zero exit, with its last stderr line;
// no response and exit 0 is ok; skipped is kept.
func TestRunBashOp_ScriptAnswers(t *testing.T) {
	tests := []struct {
		name, script string
		code         int
		status, want string
	}{
		{"failed", `echo '{"status":"failed","message":"health check failed"}'`, 1, "failed", "health check failed"},
		{"blocked", `echo '{"status":"blocked","message":"the environment is locked"}'`, 1, "blocked", "the environment is locked"},
		{"needs_input", `echo '{"status":"needs_input","message":"which region?"}'`, 1, "needs_input", "which region?"},
		{"non-zero exit", "echo 'ssh: connection refused' >&2\nexit 3", 1, "failed", "ssh: connection refused"},
		{"unknown status", `echo '{"status":"deployed"}'`, 1, "failed", `unknown status "deployed"`},
		{"no response", "echo plain output", 0, "ok", "deploy/run.sh"},
		{"skipped", `echo '{"status":"skipped","message":"disabled here"}'`, 0, "skipped", "disabled here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newBashRepo(t, map[string]string{"deploy/run.sh": tt.script + "\n"})
			code, resp, _ := r.run("deploy", r.dir, map[string]any{"script": "deploy/run.sh"})
			if code != tt.code || resp.Status != tt.status || !strings.Contains(resp.Message, tt.want) {
				t.Errorf("deploy = %d %+v, want %d %s %q", code, resp, tt.code, tt.status, tt.want)
			}
		})
	}
}

// verify and publish run their own scripts; without one they are skipped.
func TestRunBashOp_VerifyAndPublishAreOptional(t *testing.T) {
	r := newBashRepo(t, map[string]string{"deploy/run.sh": "true\n", "deploy/check.sh": `echo '{"status":"ok","message":"healthy"}'` + "\n"})
	for _, op := range []string{"verify", "publish"} {
		code, resp, _ := r.run(op, r.dir, map[string]any{"script": "deploy/run.sh", "verify_script": "", "publish_script": ""})
		if code != 0 || resp.Status != extension.StatusSkipped || !strings.Contains(resp.Message, op+"_script") {
			t.Errorf("%s without a script = %d %+v", op, code, resp)
		}
	}
	code, resp, _ := r.run("verify", r.dir, map[string]any{"script": "deploy/run.sh", "verify_script": "deploy/check.sh"})
	if code != 0 || resp.Status != extension.StatusOK || resp.Message != "healthy" {
		t.Errorf("verify = %d %+v", code, resp)
	}
}

// preflight checks, in the repository and before anything is written,
// that every configured script is a file inside it.
func TestRunBashOp_PreflightRefusesMissingOrEscapingScripts(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "evil.sh")
	if err := os.WriteFile(outside, []byte("true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		config map[string]any
		want   string
	}{
		{"no script", map[string]any{}, "config.script is required"},
		{"missing script", map[string]any{"script": "deploy/nope.sh"}, `config.script "deploy/nope.sh": no such file in the repository`},
		{"parent path", map[string]any{"script": "../evil.sh"}, `config.script "../evil.sh" must be a relative path inside the repository`},
		{"absolute path", map[string]any{"script": outside}, "must be a relative path inside the repository"},
		{"symlink out", map[string]any{"script": "deploy/link.sh"}, "resolves outside the repository"},
		{"a directory", map[string]any{"script": "deploy"}, "is not a regular file"},
		{"inside .git", map[string]any{"script": ".git/HEAD"}, "must be a relative path inside the repository"},
		{"missing verify script", map[string]any{"script": "deploy/run.sh", "verify_script": "deploy/check.sh"}, `config.verify_script "deploy/check.sh": no such file`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newBashRepo(t, map[string]string{"deploy/run.sh": "true\n"})
			if err := os.Symlink(outside, filepath.Join(r.dir, "deploy", "link.sh")); err != nil {
				t.Fatal(err)
			}
			code, resp, _ := r.run("preflight", r.dir, tt.config)
			if code != 1 || resp.Status != extension.StatusFailed || !strings.Contains(resp.Message, tt.want) {
				t.Errorf("preflight = %d %+v, want failed %q", code, resp, tt.want)
			}
		})
	}
	r := newBashRepo(t, map[string]string{"deploy/run.sh": "true\n", "deploy/check.sh": "true\n"})
	code, resp, _ := r.run("preflight", r.dir, map[string]any{"script": "./deploy/run.sh", "verify_script": "deploy/check.sh"})
	if code != 0 || resp.Status != extension.StatusOK {
		t.Errorf("valid preflight = %d %+v", code, resp)
	}
}

// deploy runs only a regular file of the release commit: a script missing
// from it, or a symlink there, is refused before bash runs.
func TestRunBashOp_DeployRefusesWhatTheReleaseCommitLacks(t *testing.T) {
	r := newBashRepo(t, map[string]string{"deploy/run.sh": "touch \"$OUT/ran\"\n"})
	if err := os.Symlink("run.sh", filepath.Join(r.dir, "deploy", "link.sh")); err != nil {
		t.Fatal(err)
	}
	r.git("-C", r.dir, "add", "-A")
	r.git("-C", r.dir, "commit", "-q", "-m", "link")
	r.git("-C", r.dir, "tag", "-f", "v1.2.0")
	r.write("deploy/new.sh", "touch \"$OUT/ran\"\n")
	for script, want := range map[string]string{
		"deploy/new.sh":  `config.script "deploy/new.sh" is not in the release commit v1.2.0`,
		"deploy/link.sh": `config.script "deploy/link.sh" is a symlink in the release commit v1.2.0`,
		"../x.sh":        "must be a relative path inside the repository",
	} {
		code, resp, _ := r.run("deploy", r.dir, map[string]any{"script": script})
		if code != 1 || resp.Status != extension.StatusFailed || !strings.Contains(resp.Message, want) {
			t.Errorf("%s: deploy = %d %+v, want %q", script, code, resp, want)
		}
	}
	if r.read("ran") != "" || fileExists(filepath.Join(r.out, "ran")) {
		t.Error("a refused script ran")
	}
	// Outside a repository holding the release tag, deploy refuses.
	code, resp, _ := r.run("deploy", t.TempDir(), map[string]any{"script": "deploy/run.sh"})
	if code != 1 || !strings.Contains(resp.Message, "release tag v1.2.0") {
		t.Errorf("deploy outside the clone = %d %+v", code, resp)
	}
}

func TestRunBashOp_RefusesUnknownOperationsAndRequests(t *testing.T) {
	var out bytes.Buffer
	if code := RunBashOp(context.Background(), "rollback", strings.NewReader("{}"), &out, BashDeps{}); code != 1 || !strings.Contains(out.String(), `unknown operation \"rollback\"`) {
		t.Errorf("unknown op: %d %s", code, out.String())
	}
	out.Reset()
	if code := RunBashOp(context.Background(), "deploy", bytes.NewReader(bashRequest("verify", nil)), &out, BashDeps{}); code != 1 || !strings.Contains(out.String(), "not deploy") {
		t.Errorf("op mismatch: %d %s", code, out.String())
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
