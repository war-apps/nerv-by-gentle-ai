package semrel

import (
	"context"
	"errors"
	"path/filepath"
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

func TestRemoteIsGitHub(t *testing.T) {
	tests := []struct {
		public, ghHost string
		want           bool
	}{
		{"https://github.com/acme/w.git", "", true},
		{"git@github.com:acme/w.git", "", true},
		{"ssh://github.com:22/acme/w.git", "", true},
		{"https://GitHub.com/acme/w", "", true},
		{"https://git.acme.io/acme/w.git", "git.acme.io", true},
		{"https://gitlab.com/acme/w.git", "", false},
		{"https://github.com.evil.io/acme/w.git", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		env := []string{}
		if tt.ghHost != "" {
			env = append(env, "GH_HOST="+tt.ghHost)
		}
		if got := remoteIsGitHub(tt.public, env); got != tt.want {
			t.Errorf("remoteIsGitHub(%q, GH_HOST=%q) = %v, want %v", tt.public, tt.ghHost, got, tt.want)
		}
	}
}

func TestCheckFixedPrereleaseGlobs_InvalidPatternIsAnError(t *testing.T) {
	err := checkFixedPrereleaseGlobs([]defaultBranch{{Name: "release/[", Prerelease: "rc"}}, []string{"release/1"})
	if err == nil || !strings.Contains(err.Error(), "release/[") {
		t.Errorf("err = %v, want an invalid-pattern error", err)
	}
}

func TestWorkspaceRelease_BuiltInConfigurationRequest(t *testing.T) {
	tests := []struct {
		name, origin string
		heads        []string
		wantGitHub   bool
		wantErr      error
	}{
		{"github origin", "https://github.com/acme/w.git", []string{"main", "develop"}, true, nil},
		{"other origin", "https://gitlab.com/acme/w.git", []string{"main", "develop"}, false, nil},
		{"two release branches on origin", "https://github.com/acme/w.git", []string{"main", "release/1.1", "release/1.2"}, false, ErrFixedPrereleaseGlob},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			var runs []runnerRequest
			f := workspaceRunner(t, root, tt.origin, "abc", func(req runnerRequest) (Result, error) {
				if req.Mode == "plan" {
					return respond(map[string]any{"ok": true, "packages": []string{}, "configFile": ""})
				}
				runs = append(runs, req)
				return respond(map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "1.1.0", "gitTag": "v1.1.0"}})
			})
			base := f.handle
			f.handle = func(c Command) (Result, error) {
				if isGit(c, "for-each-ref") {
					var b strings.Builder
					for _, h := range tt.heads {
						b.WriteString("1111111111111111111111111111111111111111 refs/remotes/origin/" + h + "\n")
					}
					return Result{Stdout: []byte(b.String())}, nil
				}
				return base(c)
			}
			ws := openWorkspace(t, f, root)
			r, err := ws.Release(context.Background(), ReleaseRequest{Expected: ProjectResult{Project: filepath.Base(root), NewReleasePublished: true, NewReleaseVersion: "1.1.0"}, ExtraAssets: []string{"src/Api/Api.csproj"}})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || len(runs) != 0 {
					t.Fatalf("err = %v, runs %d", err, len(runs))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) != 1 || !runs[0].UseDefault || runs[0].GitHubRelease != tt.wantGitHub || r.SemanticReleaseConfig != ConfigBuiltIn || !slices.Equal(runs[0].ExtraAssets, []string{"src/Api/Api.csproj"}) {
				t.Errorf("runs %+v, result %+v", runs, r)
			}
		})
	}
}

func TestWorkspaceRelease_ExtraAssetsOnlyForTheBuiltInConfiguration(t *testing.T) {
	root := t.TempDir()
	var runs []runnerRequest
	f := workspaceRunner(t, root, "https://github.com/acme/w.git", "abc", func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return respond(map[string]any{"ok": true, "packages": []string{}, "configFile": ".releaserc.json"})
		}
		runs = append(runs, req)
		return respond(map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "1.1.0", "gitTag": "v1.1.0"}})
	})
	ws := openWorkspace(t, f, root)
	if _, err := ws.Release(context.Background(), ReleaseRequest{Expected: ProjectResult{Project: filepath.Base(root), NewReleasePublished: true, NewReleaseVersion: "1.1.0"}, ExtraAssets: []string{"a.csproj"}}); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].UseDefault || runs[0].ExtraAssets != nil {
		t.Errorf("request %+v: the repository's own configuration keeps its own assets", runs)
	}
}

func TestAssetsCover(t *testing.T) {
	tests := []struct {
		assets []string
		file   string
		want   bool
	}{
		{[]string{"src/Api/Api.csproj"}, "src/Api/Api.csproj", true},
		{[]string{"./src/Api/Api.csproj"}, "src/Api/Api.csproj", true},
		{[]string{"src/Api/CHANGELOG.md", "package.json"}, "src/Api/Api.csproj", false},
		{[]string{"src/**/*.csproj"}, "src/Api/Api.csproj", true},
		{[]string{"**/*.csproj"}, "Api.csproj", true},
		{[]string{"*.csproj"}, "src/Api/Api.csproj", false},
		{[]string{"src/Api/*.{csproj,props}"}, "src/Api/Api.csproj", true},
		{[]string{"src/Api/Api.cs?roj"}, "src/Api/Api.csproj", true},
		{[]string{"src/**", "!src/Api/Api.csproj"}, "src/Api/Api.csproj", false},
		{nil, "src/Api/Api.csproj", false},
	}
	for _, tt := range tests {
		if got := AssetsCover(tt.assets, tt.file); got != tt.want {
			t.Errorf("AssetsCover(%q, %q) = %v, want %v", tt.assets, tt.file, got, tt.want)
		}
	}
}

func TestCompute_ReportsTheRepositorysGitAssets(t *testing.T) {
	root := t.TempDir()
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
		if req.Mode == "plan" {
			return respond(map[string]any{"ok": true, "packages": []string{}, "configFile": ".releaserc.json", "gitPlugin": true, "gitAssets": []string{"CHANGELOG.md", "src/**/*.csproj"}})
		}
		return respond(map[string]any{"ok": true, "released": true, "nextRelease": map[string]any{"version": "1.1.0", "gitTag": "v1.1.0"}})
	})
	results, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir(), Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := results[0]; !r.GitPlugin || !slices.Equal(r.GitAssets, []string{"CHANGELOG.md", "src/**/*.csproj"}) {
		t.Errorf("result %+v", r)
	}
}
