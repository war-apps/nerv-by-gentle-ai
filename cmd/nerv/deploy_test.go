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

// The hidden provider subcommand answers with a JSON response.
func TestRunDeploySSHCompose_AnswersJSON(t *testing.T) {
	opts := testOptions(t.TempDir())
	opts.Stdin = strings.NewReader(`{"operation":"deploy","config":{}}`)
	var stdout, stderr bytes.Buffer
	code := run([]string{"deploy-ssh-compose", "deploy"}, &stdout, &stderr, opts)
	var resp map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("stdout %q: %v", stdout.String(), err)
	}
	if code != 1 || resp["status"] != "failed" || !strings.Contains(resp["message"], "config.environment is required") {
		t.Errorf("exit %d, response %v", code, resp)
	}
	if strings.Contains(usage, "deploy-ssh-compose") || !strings.Contains(usage, "deploy --artifact") {
		t.Errorf("usage must list deploy and hide deploy-ssh-compose:\n%s", usage)
	}
}
