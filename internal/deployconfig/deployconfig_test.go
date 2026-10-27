package deployconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, content string) (*Deploy, error) {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".ai-workflow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".ai-workflow/config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(repo)
}

func TestLoad_ReadsTheDeploySectionWithDefaults(t *testing.T) {
	d, err := load(t, `tasks:
  provider: teamwork
deploy:
  host: github
  targets:
    - name: staging
      provider: ssh-compose
      config:
        environment: staging
        ssh_port: 2222
    - name: prod
      provider: ssh-compose
`)
	if err != nil {
		t.Fatal(err)
	}
	if d.Host != "github" || len(d.Targets) != 2 || d.Targets[0].Config["ssh_port"] != "2222" || d.Targets[1].Name != "prod" {
		t.Errorf("deploy = %+v", d)
	}
	if d.Git.Remote != "origin" || d.Git.CommitMessage != DefaultCommitMessage {
		t.Errorf("git defaults = %+v", d.Git)
	}
	if DefaultCommitMessage != "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}" {
		t.Errorf("default message %q is not @semantic-release/git's", DefaultCommitMessage)
	}
}

func TestLoad_NotConfigured(t *testing.T) {
	if _, err := load(t, "tasks:\n  provider: teamwork\n"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no deploy: %v", err)
	}
	if _, err := Load(t.TempDir()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no file: %v", err)
	}
}

func TestLoad_RefusesUnknownKeysAndInvalidValues(t *testing.T) {
	tests := []struct{ name, section, want string }{
		{"unknown key", "deploy:\n  host: github\n  hosts: [x]\n", "deploy.hosts: unknown field"},
		{"unknown target key", "deploy:\n  targets:\n    - name: a\n      provider: p\n      env: x\n", "deploy.targets[0].env: unknown field"},
		{"unknown git key", "deploy:\n  git:\n    sign: true\n", "deploy.git.sign: unknown field"},
		{"duplicate target", "deploy:\n  targets:\n    - {name: a, provider: p}\n    - {name: a, provider: p}\n", "deploy.targets[1].name: \"a\" is also targets[0]"},
		{"bad target name", "deploy:\n  targets:\n    - {name: A B, provider: p}\n", "deploy.targets[0].name"},
		{"missing provider", "deploy:\n  targets:\n    - {name: a}\n", "deploy.targets[0].provider: required"},
		{"bad host", "deploy:\n  host: Git Hub\n", "deploy.host"},
		{"bad placeholder", "deploy:\n  git:\n    commit_message: \"release ${nextRelease.name}\"\n", "unknown placeholder ${nextRelease.name}"},
		{"option remote", "deploy:\n  git:\n    remote: --upload-pack=x\n", "deploy.git.remote"},
		{"bad config key", "deploy:\n  targets:\n    - name: a\n      provider: p\n      config: {Port: 1}\n", "deploy.targets[0].config.Port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(t, tt.section)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load: %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCommitMessage_RendersSemanticReleasePlaceholders(t *testing.T) {
	got := CommitMessage(DefaultCommitMessage, Release{Version: "1.2.0", Tag: "v1.2.0", Notes: "## 1.2.0\n\n* x\n"})
	if want := "chore(release): 1.2.0 [skip ci]\n\n## 1.2.0\n\n* x\n"; got != want {
		t.Errorf("message %q, want %q", got, want)
	}
	got = CommitMessage("release ${version} (${tag}) on ${channel}", Release{Version: "2.0.0-rc.1", Tag: "v2.0.0-rc.1", Channel: "beta", Notes: "${version}"})
	if got != "release 2.0.0-rc.1 (v2.0.0-rc.1) on beta" {
		t.Errorf("short placeholders: %q", got)
	}
	if got := CommitMessage("n: ${nextRelease.notes}", Release{Notes: "uses ${nextRelease.version}"}); got != "n: uses ${nextRelease.version}" {
		t.Errorf("notes are not re-expanded: %q", got)
	}
}

// S9: the old pipeline: section is refused, saying where its deploy went.
func TestLoad_RefusesTheOldPipelineSection(t *testing.T) {
	_, err := load(t, "pipeline:\n  deploy:\n    selection: branches\ndeploy:\n  targets: []\n")
	if err == nil || !strings.Contains(err.Error(), "deploy: the release-ssh-compose target") || !strings.Contains(err.Error(), "selection: branches") {
		t.Fatalf("Load = %v", err)
	}
	// Without a deploy: section the refusal still comes first.
	if _, err := load(t, "pipeline:\n  test: { run: x }\n"); err == nil || errors.Is(err, ErrNotConfigured) || !strings.Contains(err.Error(), "pipeline:") {
		t.Fatalf("Load without deploy: = %v", err)
	}
}

// deploy.publish defaults to true; false is kept; a non-boolean is refused.
func TestLoad_Publish(t *testing.T) {
	d, err := load(t, "deploy:\n  host: github\n")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Publishes() || len(d.Targets) != 0 {
		t.Errorf("publish %v, targets %v: want the default true and no target", d.Publishes(), d.Targets)
	}
	if d, err = load(t, "deploy:\n  host: github\n  publish: false\n"); err != nil || d.Publishes() {
		t.Errorf("publish: false read as %v (%v)", d != nil && d.Publishes(), err)
	}
	if _, err := load(t, "deploy:\n  publish: maybe\n"); err == nil || !strings.Contains(err.Error(), "into bool") {
		t.Errorf("publish: maybe: %v, want refused", err)
	}
}
