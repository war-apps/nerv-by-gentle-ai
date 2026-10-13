package semrel

import (
	"context"
	"strings"
	"testing"
)

func TestParseGitVersion(t *testing.T) {
	tests := []struct {
		out     string
		want    [3]int
		wantErr bool
	}{
		{out: "git version 2.43.0\n", want: [3]int{2, 43, 0}},
		{out: "git version 2.39.3 (Apple Git-146)", want: [3]int{2, 39, 3}},
		{out: "git version 2.45.1.windows.1", want: [3]int{2, 45, 1}},
		{out: "git version 2.31.0", want: [3]int{2, 31, 0}},
		{out: "git version 2.30", want: [3]int{2, 30, 0}},
		{out: "git version 3.0.0-rc0", want: [3]int{3, 0, 0}},
		{out: "", wantErr: true},
		{out: "not git", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseGitVersion(tt.out)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseGitVersion(%q) error %v, wantErr %v", tt.out, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("parseGitVersion(%q) = %v, want %v", tt.out, got, tt.want)
		}
	}
}

// gitVersionRepo is fakeRepo whose git --version prints version.
func gitVersionRepo(t *testing.T, root, version string, ran *int) *fakeRunner {
	f := fakeRepo(t, root, func(req runnerRequest) (Result, error) {
		*ran++
		if req.Mode == "plan" {
			return okPlan()
		}
		return respond(map[string]any{"ok": true, "released": false})
	})
	inner := f.handle
	f.handle = func(c Command) (Result, error) {
		if c.Name == "git" && len(c.Args) == 1 && c.Args[0] == "--version" {
			return Result{Stdout: []byte(version + "\n")}, nil
		}
		return inner(c)
	}
	return f
}

func TestCompute_PathFilterNeedsGit231(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `release:
  projects:
    - { name: api, path: api }
    - { name: web, path: web }
`)
	ran := 0
	f := gitVersionRepo(t, root, "git version 2.30.2", &ran)
	_, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()})
	if err == nil {
		t.Fatal("want an error for git 2.30.2")
	}
	for _, want := range []string{"2.30.2", "2.31.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
	if ran != 0 {
		t.Errorf("the runner ran %d times before the git version check failed", ran)
	}
}

func TestCompute_GitVersionCheckedOnlyForPathFilters(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `release:
  projects:
    - { name: app, path: . }
`)
	ran := 0
	f := gitVersionRepo(t, root, "git version 2.30.2", &ran)
	if _, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()}); err != nil {
		t.Fatalf("a project without a path filter must not need git 2.31: %v", err)
	}
	if calls := f.find(func(c Command) bool { return isGit(c, "--version") }); len(calls) != 0 {
		t.Errorf("git --version called %d times without a path filter", len(calls))
	}

	ran = 0
	writeConfig(t, root, `release:
  projects:
    - { name: api, path: api }
`)
	f = gitVersionRepo(t, root, "git version 2.39.3 (Apple Git-146)", &ran)
	if _, err := (&Engine{Runner: f}).Compute(context.Background(), Options{RepoRoot: root, Branch: "main", CacheDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if calls := f.find(func(c Command) bool { return isGit(c, "--version") }); len(calls) != 1 {
		t.Errorf("git --version called %d times, want once", len(calls))
	}
}
