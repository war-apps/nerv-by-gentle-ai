package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setPipelineSecrets puts the pipeline's secrets in nerv's own process
// environment, as in a real run, plus a variable that must be kept.
func setPipelineSecrets(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"GHCR_USER": "bot", "GHCR_TOKEN": "registry-secret", "GH_TOKEN": "gh-secret", "GITHUB_TOKEN": "gh-secret-2",
		"NERV_DEPLOY_STAGING_DOCKER_SSH_PASS": "ssh-secret", "NERV_DEPLOY_STAGING_DOCKER_SSH_HOST": "10.0.0.1", "KEEP_ME": "kept",
	} {
		t.Setenv(k, v)
	}
}

// secretsIn returns the secret variable names found in env.
func secretsIn(env []string) []string {
	var found []string
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		switch {
		case name == "GHCR_USER" || name == "GHCR_TOKEN" || strings.HasPrefix(name, "NERV_DEPLOY_"):
			found = append(found, name)
		}
	}
	return found
}

// TestPipeline_DotnetNeverGetsTheSecrets runs validate and test with the
// real process runner and a fake dotnet on PATH that records its
// environment: the registry and deploy secrets and the GitHub tokens must
// not reach it (it builds and runs the repository's code).
func TestPipeline_DotnetNeverGetsTheSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake dotnet is a shell script")
	}
	bin, dump := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\nenv > " + dump + "/env.$$\nif [ \"$1\" = --version ]; then echo 10.0.112; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "dotnet"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	setPipelineSecrets(t)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	f := newPipelineFixture(t, pipelineYAML)
	for _, step := range []string{"validate", "test"} {
		opts := testOptions(t.TempDir())
		opts.Runner = f.runner
		var out, errOut bytes.Buffer
		if code := run([]string{"pipeline", step}, &out, &errOut, opts); code != 0 {
			t.Fatalf("%s: exit %d\n%s", step, code, errOut.String())
		}
	}
	files, _ := filepath.Glob(filepath.Join(dump, "env.*"))
	if len(files) < 5 {
		t.Fatalf("dotnet ran %d times, want the probe and every command", len(files))
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		env := strings.Split(strings.TrimSpace(string(data)), "\n")
		if found := secretsIn(env); len(found) > 0 {
			t.Errorf("dotnet's environment holds %v", found)
		}
		for _, kv := range env {
			if strings.HasPrefix(kv, "GH_TOKEN=") || strings.HasPrefix(kv, "GITHUB_TOKEN=") {
				t.Errorf("dotnet's environment holds %s", strings.SplitN(kv, "=", 2)[0])
			}
		}
		if !strings.Contains(string(data), "KEEP_ME=kept") || !strings.Contains(string(data), "PATH=") {
			t.Errorf("dotnet lost the rest of the environment:\n%s", data)
		}
	}
}

func TestPipelineRelease_SemanticReleaseNeverGetsTheDeployOrRegistrySecrets(t *testing.T) {
	setPipelineSecrets(t)
	f := newPipelineReleaseFixture(t, pipelineReleaseYAML)
	if code, _, stderr := f.runPipeline(t, "release", "--yes", "--json"); code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	env := f.opened[0].Env
	if env == nil || len(secretsIn(env)) > 0 || !strings.Contains(strings.Join(env, "\n"), "GH_TOKEN=gh-secret") {
		t.Errorf("release clone environment: secrets %v (GH_TOKEN kept for the github plugin: %v)", secretsIn(env), strings.Contains(strings.Join(env, "\n"), "GH_TOKEN="))
	}
}

func TestPipelineSync_GitNeverGetsTheDeployOrRegistrySecrets(t *testing.T) {
	setPipelineSecrets(t)
	f := newSyncFixture(t, pipelineSyncYAML)
	f.release(t, mainRelease())
	if code, _, stderr := f.run(t, "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if env := f.ws.opened.Env; env == nil || len(secretsIn(env)) > 0 {
		t.Errorf("sync clone environment: secrets %v (nil env: %v)", secretsIn(env), env == nil)
	}
}

func TestPipelineDocker_GitNeverGetsTheDeployOrRegistrySecrets(t *testing.T) {
	setPipelineSecrets(t)
	f := newDockerFixture(t, pipelineDockerYAML)
	f.release(t, alphaRelease())
	if code, _, stderr := f.run(t, "--json"); code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if env := f.ws.opened.Env; env == nil || len(secretsIn(env)) > 0 {
		t.Errorf("docker clone environment: secrets %v (nil env: %v)", secretsIn(env), env == nil)
	}
}
