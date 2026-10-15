package semrel

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
)

// defaultRepo is a fake repository whose plan mode reports configFile
// ("" = no semantic-release configuration of its own) and whose mirror holds
// heads.
func defaultRepo(t *testing.T, root, configFile string, heads []string, runs *[]runnerRequest) *fakeRunner {
	t.Helper()
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return respond(map[string]any{"ok": true, "packages": []string{"@semantic-release/commit-analyzer"}, "configFile": configFile})
		}
		*runs = append(*runs, req)
		return respond(map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "1.1.0-alpha.1", "gitTag": "v1.1.0-alpha.1"}})
	})
	base := f.handle
	f.handle = func(c Command) (Result, error) {
		if isGit(c, "for-each-ref") {
			var b strings.Builder
			for _, h := range heads {
				b.WriteString("1111111111111111111111111111111111111111 refs/heads/" + h + "\n")
			}
			return Result{Stdout: []byte(b.String())}, nil
		}
		return base(c)
	}
	return f
}

func TestCompute_BuiltInDefaultWhenTheRepositoryHasNoConfig(t *testing.T) {
	root := t.TempDir()
	var runs []runnerRequest
	f := defaultRepo(t, root, "", []string{"main", "develop"}, &runs)
	results, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "develop", CacheDir: t.TempDir(), Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || !runs[0].UseDefault {
		t.Fatalf("runs %+v: want the built-in default", runs)
	}
	want := pipelineconfig.DefaultBranches()
	for i := range want {
		if want[i].Channel == "" {
			want[i].Channel = want[i].Prerelease
		}
	}
	if len(runs[0].Branches) != len(want) {
		t.Fatalf("branches %+v, want %+v", runs[0].Branches, want)
	}
	for i, b := range runs[0].Branches {
		if b != (defaultBranch{Name: want[i].Name, Prerelease: want[i].Prerelease, Channel: want[i].Channel}) {
			t.Errorf("branch %d = %+v, want %+v", i, b, want[i])
		}
	}
	if results[0].SemanticReleaseConfig != ConfigBuiltIn || results[0].SemanticReleaseConfigFile != "" {
		t.Errorf("source %q %q", results[0].SemanticReleaseConfig, results[0].SemanticReleaseConfigFile)
	}
}

func TestCompute_PipelineBranchesReplaceTheDefaultOnes(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `pipeline:
  dotnet:
    solution: App.slnx
  images:
    - name: ghcr.io/acme/app
  branches:
    - name: master
    - name: develop
      prerelease: dev
    - name: release/*
      prerelease: rc
      channel: next
`)
	var runs []runnerRequest
	f := defaultRepo(t, root, "", []string{"master", "develop"}, &runs)
	if _, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "develop", CacheDir: t.TempDir(), Offline: true}); err != nil {
		t.Fatal(err)
	}
	want := []defaultBranch{{Name: "master"}, {Name: "develop", Prerelease: "dev", Channel: "dev"}, {Name: "release/*", Prerelease: "rc", Channel: "next"}}
	if len(runs) != 1 || !slices.Equal(runs[0].Branches, want) {
		t.Errorf("branches %+v, want %+v", runs, want)
	}
}

func TestCompute_RepositoryConfigIsKept(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "pipeline:\n  dotnet:\n    solution: App.slnx\n  images:\n    - name: ghcr.io/acme/app\n")
	var runs []runnerRequest
	f := defaultRepo(t, root, ".releaserc.json", []string{"main"}, &runs)
	results, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir(), Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].UseDefault || runs[0].Branches != nil {
		t.Errorf("request %+v: the repository's own configuration must be used as it is", runs[0])
	}
	if results[0].SemanticReleaseConfig != ConfigRepository || results[0].SemanticReleaseConfigFile != ".releaserc.json" {
		t.Errorf("source %q %q", results[0].SemanticReleaseConfig, results[0].SemanticReleaseConfigFile)
	}
}

func TestCompute_FixedPrereleaseGlobMatchingSeveralBranchesIsRefused(t *testing.T) {
	root := t.TempDir()
	var runs []runnerRequest
	f := defaultRepo(t, root, "", []string{"main", "release/1.1", "release/1.2"}, &runs)
	_, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "release/1.1", CacheDir: t.TempDir(), Offline: true})
	if !errors.Is(err, ErrFixedPrereleaseGlob) || !strings.Contains(err.Error(), "release/1.1, release/1.2") || !strings.Contains(err.Error(), `"rc"`) {
		t.Fatalf("err = %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("semantic-release ran: %+v", runs)
	}
}

func TestCheckFixedPrereleaseGlobs(t *testing.T) {
	branches := []defaultBranch{{Name: "main"}, {Name: "release/*", Prerelease: "rc", Channel: "beta"}, {Name: "hotfix/*", Prerelease: "beta", Channel: "beta"}}
	tests := []struct {
		name  string
		heads []string
		bad   bool
	}{
		{"none", []string{"main"}, false},
		{"one release branch", []string{"main", "release/1.0"}, false},
		{"one of each glob", []string{"main", "release/1.0", "hotfix/x"}, false},
		{"nested names do not match *", []string{"release/1.0", "release/a/b"}, false},
		{"two hotfix branches", []string{"hotfix/a", "hotfix/b"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkFixedPrereleaseGlobs(branches, tt.heads); (err != nil) != tt.bad {
				t.Errorf("err = %v, want error %v", err, tt.bad)
			}
		})
	}
}
