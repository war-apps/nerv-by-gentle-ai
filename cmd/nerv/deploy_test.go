package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
)

func TestRunDeploy_RequiresTheArtifact(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"deploy"}, &stdout, &stderr, testOptions(t.TempDir())); code != 2 || !strings.Contains(stderr.String(), "--artifact is required") {
		t.Errorf("exit %d, stderr %s", code, stderr.String())
	}
	stdout.Reset()
	if code := run([]string{"deploy", "--help"}, &stdout, &stderr, testOptions(t.TempDir())); code != 0 || !strings.Contains(stdout.String(), "Usage: nerv deploy --artifact") {
		t.Errorf("help: exit %d\n%s", code, stdout.String())
	}
}

// The help says plainly what a target's preflight checks.
func TestRunDeploy_HelpExplainsThePreflight(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"deploy", "--help"}, &stdout, &stderr, testOptions(t.TempDir())); code != 0 {
		t.Fatalf("help: exit %d", code)
	}
	help := strings.Join(strings.Fields(stdout.String()), " ")
	want := "Before anything is cloned or pushed, each target's preflight checks the target's config and that the tools its provider needs are installed (deploy-ssh-compose: sshpass for password authentication, infisical for source infisical)."
	if !strings.Contains(help, want) {
		t.Errorf("help lacks %q:\n%s", want, stdout.String())
	}
}

// A repository without a deploy: section is refused (exit 1) before
// anything runs.
func TestRunDeploy_RefusesAnUnconfiguredRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	opts := testOptions(t.TempDir())
	opts.Runner = env.ExecRunner{}
	opts.ExtensionsFS = nerv.ExtensionsFS()
	var stdout, stderr bytes.Buffer
	code := run([]string{"deploy", "--cwd", repo, "--artifact", filepath.Join(repo, "x"), "--json"}, &stdout, &stderr, opts)
	if code != 1 || !strings.Contains(stderr.String(), "no deploy: section") {
		t.Fatalf("exit %d, stderr %s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil || !strings.Contains(out["error"].(string), "deploy:") {
		t.Errorf("json report %q: %v", stdout.String(), err)
	}
}

// The hidden native provider command answers with a JSON response, for
// every operation of the deploy-ssh-compose provider.
func TestRunProvider_DeploySSHComposeAnswersJSON(t *testing.T) {
	for _, op := range []string{"preflight", "deploy", "verify"} {
		opts := testOptions(t.TempDir())
		opts.Stdin = strings.NewReader(`{"contract":1,"extension":"deploy","operation":"` + op + `","config":{}}`)
		var stdout, stderr bytes.Buffer
		code := run([]string{"provider", "deploy-ssh-compose", op}, &stdout, &stderr, opts)
		var resp map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
			t.Fatalf("%s: stdout %q: %v", op, stdout.String(), err)
		}
		if msg, _ := resp["message"].(string); code != 1 || resp["status"] != "failed" || !strings.Contains(msg, "config.environment is required") {
			t.Errorf("%s: exit %d, response %v", op, code, resp)
		}
	}
	opts := testOptions(t.TempDir())
	// nerv deploy sends the config with the manifest's defaults applied.
	opts.Stdin = strings.NewReader(`{"operation":"preflight","config":{"environment":"staging","ssh_auth":"password","source":"infisical",` +
		`"infisical_project_id":"p","infisical_app_path":"/a","ssh_known_hosts":"~/.ssh/known_hosts","ssh_port":"22","ssh_connect_timeout":"10s",` +
		`"update_timeout":"15m","verify_healthy":"true","version_label":"org.opencontainers.image.version","verify_timeout":"120s"}}`)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"provider", "deploy-ssh-compose", "preflight"}, &stdout, &stderr, opts); code != 0 ||
		!strings.Contains(stdout.String(), `"data":{"requires":["sshpass","infisical"]}`) {
		t.Errorf("preflight: exit %d, stdout %s", code, stdout.String())
	}
	if strings.Contains(usage, "provider deploy-ssh-compose") || strings.Contains(usage, "deploy-ssh-compose <") || !strings.Contains(usage, "deploy --artifact") {
		t.Errorf("usage must list deploy and hide the provider command:\n%s", usage)
	}
}

// Only nerv's native providers run under nerv provider; the old top-level
// deploy-ssh-compose command is gone.
func TestRunProvider_RefusesAnUnknownProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"provider", "deploy-github", "createRelease"}, &stdout, &stderr, testOptions(t.TempDir())); code != 2 ||
		!strings.Contains(stderr.String(), `"deploy-github" is not a native provider (deploy-bash, deploy-ssh-compose, release-bash, release-conventional-commits, release-docker-compose)`) {
		t.Errorf("exit %d, stderr %s", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"provider", "deploy-ssh-compose"}, &stdout, &stderr, testOptions(t.TempDir())); code != 2 || !strings.Contains(stderr.String(), "usage: nerv provider") {
		t.Errorf("exit %d, stderr %s", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"deploy-ssh-compose", "deploy"}, &stdout, &stderr, testOptions(t.TempDir())); code == 0 {
		t.Errorf("the removed deploy-ssh-compose command still runs: exit %d", code)
	}
}

// S4, S6: the release providers are native exec providers: the hidden nerv
// provider release-<name> <operation> reads the common request on stdin
// and prints the common response on stdout.
func TestRunProvider_RunsTheNativeReleaseProviders(t *testing.T) {
	call := func(plugin, op, request string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		opts := testOptions(t.TempDir())
		opts.Stdin = strings.NewReader(request)
		code := run([]string{"provider", plugin, op}, &stdout, &stderr, opts)
		return code, stdout.String(), stderr.String()
	}
	code, out, _ := call("release-docker-compose", "plan", `{"contract":1,"extension":"release","operation":"plan","config":{"file":"docker-compose.yml"},"payload":{"project":"api","at":"release.projects[0]","stages":[]}}`)
	if code != 0 || !strings.Contains(out, `"status":"ok"`) || !strings.Contains(out, `"exec":["docker","compose","-f","docker-compose.yml","build"]`) {
		t.Errorf("plan: exit %d, stdout %s", code, out)
	}
	code, out, _ = call("release-conventional-commits", "validate", `{"contract":1,"extension":"release","operation":"validate","config":{"tag_format":"v1"},"payload":{"projects":[]}}`)
	if code != 1 || !strings.Contains(out, `"status":"failed"`) || !strings.Contains(out, `release.providers.conventional-commits.tag_format`) {
		t.Errorf("validate: exit %d, stdout %s", code, out)
	}
	if code, _, stderr := call("release-bash", "plan", "not json"); code != 2 || !strings.Contains(stderr, "not the JSON envelope") {
		t.Errorf("bad request: exit %d, stderr %s", code, stderr)
	}
}
