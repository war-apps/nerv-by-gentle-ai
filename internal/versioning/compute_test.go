package versioning

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
)

// Rule 1: commit-analyzer's default rules with the conventionalcommits
// preset and dim's note keywords.
func TestComputeBumpRules(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    string // next version; "" for no release
	}{
		{"feat is minor", "feat: add a thing", "1.1.0"},
		{"fix is patch", "fix: repair a thing", "1.0.1"},
		{"perf is patch", "perf: speed a thing", "1.0.1"},
		{"bang is major", "feat(api)!: drop a thing", "2.0.0"},
		{"bang on a hidden type is major", "chore!: drop node 18", "2.0.0"},
		{"BREAKING CHANGE footer is major", "fix: x\n\nBREAKING CHANGE: the api moved", "2.0.0"},
		{"BREAKING CHANGES footer is major", "fix: x\n\nBREAKING CHANGES: the api moved", "2.0.0"},
		{"BREAKING keyword is major", "feat: x\n\nBREAKING: gone", "2.0.0"},
		{"keywords are case-insensitive", "fix: x\n\nbreaking change: gone", "2.0.0"},
		{"docs does not release", "docs: explain", ""},
		{"chore does not release", "chore: tidy", ""},
		{"non conventional does not release", "Update readme", ""},
		{"uppercase FEAT is minor (default JSHint rule)", "FEAT: shout", "1.1.0"},
		{"Feat is not feat", "Feat: mixed case", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newTestRepo(t)
			r.commit("feat: initial")
			r.release("v1.0.0", "")
			r.commit(c.message)
			got := one(t, r.compute(Request{}))
			if got.NewReleaseVersion != c.want || got.NewReleasePublished != (c.want != "") {
				t.Fatalf("version = %q published=%v, want %q (reason %q)", got.NewReleaseVersion, got.NewReleasePublished, c.want, got.Reason)
			}
		})
	}
}

// Rule 1 (revert): a revert commit follows commit-analyzer's default rule
// (patch), and a revert with the commit it reverts in the same range cancels
// both (conventional-commits-filter).
func TestComputeRevert(t *testing.T) {
	t.Run("revert of a released commit is patch", func(t *testing.T) {
		r := newTestRepo(t)
		feat := r.commit("feat: risky")
		r.release("v1.0.0", "")
		r.commit("Revert \"feat: risky\"\n\nThis reverts commit " + feat + ".")
		got := one(t, r.compute(Request{}))
		if got.NewReleaseVersion != "1.0.1" {
			t.Fatalf("version = %q, want 1.0.1 (reason %q)", got.NewReleaseVersion, got.Reason)
		}
	})
	t.Run("revert and reverted in the same range cancel", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("chore: initial")
		r.release("v1.0.0", "")
		feat := r.commit("feat: risky")
		r.commit("Revert \"feat: risky\"\n\nThis reverts commit " + feat + ".")
		got := one(t, r.compute(Request{}))
		if got.NewReleasePublished {
			t.Fatalf("released %q, want no release", got.NewReleaseVersion)
		}
	})
}

// Rule 1 (release rules): custom rules come first; a commit no custom rule
// matches falls back to the defaults; none stops a release.
func TestComputeReleaseRules(t *testing.T) {
	rules := []ReleaseRule{
		{Type: "docs", Scope: "readme", Release: ReleasePatch},
		{Type: "feat", Scope: "wip", Release: ReleaseNone},
		{Type: "refactor", Release: ReleaseMinor},
	}
	cases := []struct{ message, want string }{
		{"docs(readme): typo", "1.0.1"},
		{"docs(api): typo", ""},
		{"feat(wip): half done", ""},
		{"feat(api): done", "1.1.0"},
		{"refactor: inline", "1.1.0"},
		{"fix: default rule still applies", "1.0.1"},
	}
	for _, c := range cases {
		t.Run(c.message, func(t *testing.T) {
			r := newTestRepo(t)
			r.commit("feat: initial")
			r.release("v1.0.0", "")
			r.commit(c.message)
			got := one(t, r.compute(Request{Config: Config{ReleaseRules: rules}}))
			if got.NewReleaseVersion != c.want {
				t.Fatalf("version = %q, want %q", got.NewReleaseVersion, c.want)
			}
		})
	}
}

// Rule 2: release and prerelease branches, channels and prerelease
// numbering as semantic-release computes them.
func TestComputeBranches(t *testing.T) {
	t.Run("develop is alpha on the alpha channel, numbered over its tags", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: initial")
		r.release("v1.0.0", "")
		r.git("checkout", "-q", "-b", "develop")
		r.commit("feat: one")
		got := one(t, r.compute(Request{}))
		if got.NewReleaseVersion != "1.1.0-alpha.1" || got.NewReleaseChannel != "alpha" || got.NewReleaseGitTag != "v1.1.0-alpha.1" {
			t.Fatalf("got %q on %q tag %q, want 1.1.0-alpha.1 on alpha", got.NewReleaseVersion, got.NewReleaseChannel, got.NewReleaseGitTag)
		}
		r.release("v1.1.0-alpha.1", "alpha")
		r.commit("fix: two")
		got = one(t, r.compute(Request{}))
		if got.NewReleaseVersion != "1.1.0-alpha.2" || got.LastReleaseVersion != "1.1.0-alpha.1" {
			t.Fatalf("got %q after %q, want 1.1.0-alpha.2 after 1.1.0-alpha.1", got.NewReleaseVersion, got.LastReleaseVersion)
		}
		r.release("v1.1.0-alpha.2", "alpha")
		r.commit("feat!: three")
		got = one(t, r.compute(Request{}))
		if got.NewReleaseVersion != "2.0.0-alpha.1" {
			t.Fatalf("major on alpha = %q, want 2.0.0-alpha.1", got.NewReleaseVersion)
		}
	})
	t.Run("a prerelease tag without its channel note is not the last release", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: initial")
		r.release("v1.0.0", "")
		r.git("checkout", "-q", "-b", "develop")
		r.commit("feat: one")
		r.git("tag", "v1.1.0-alpha.1")
		r.commit("fix: two")
		got := one(t, r.compute(Request{}))
		if got.LastReleaseVersion != "1.0.0" || got.NewReleaseVersion != "1.1.0-alpha.1" {
			t.Fatalf("got %q after %q, want 1.1.0-alpha.1 after 1.0.0", got.NewReleaseVersion, got.LastReleaseVersion)
		}
	})
	t.Run("release/* is rc on the beta channel", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: initial")
		r.release("v1.0.0", "")
		r.git("checkout", "-q", "-b", "release/1.1")
		r.commit("fix: candidate")
		got := one(t, r.compute(Request{}))
		if got.NewReleaseVersion != "1.0.1-rc.1" || got.NewReleaseChannel != "beta" {
			t.Fatalf("got %q on %q, want 1.0.1-rc.1 on beta", got.NewReleaseVersion, got.NewReleaseChannel)
		}
	})
	t.Run("hotfix/* is beta", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: initial")
		r.release("v1.0.0", "")
		r.git("checkout", "-q", "-b", "hotfix/crash")
		r.commit("fix: crash")
		got := one(t, r.compute(Request{}))
		if got.NewReleaseVersion != "1.0.1-beta.1" || got.NewReleaseChannel != "beta" {
			t.Fatalf("got %q on %q, want 1.0.1-beta.1 on beta", got.NewReleaseVersion, got.NewReleaseChannel)
		}
	})
	t.Run("main uses the highest stable tag, ignoring merged prereleases", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: initial")
		r.release("v1.0.0", "")
		r.commit("feat: two")
		r.release("v1.1.0-alpha.1", "alpha")
		r.commit("fix: three")
		got := one(t, r.compute(Request{}))
		if got.LastReleaseVersion != "1.0.0" || got.NewReleaseVersion != "1.1.0" || got.NewReleaseChannel != "" {
			t.Fatalf("got %q after %q on %q, want 1.1.0 after 1.0.0 on the default channel", got.NewReleaseVersion, got.LastReleaseVersion, got.NewReleaseChannel)
		}
	})
	t.Run("prerelease true uses the branch name; a later release branch has its own channel", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: initial")
		r.release("v1.0.0", "")
		r.git("branch", "next")
		r.git("checkout", "-q", "-b", "beta")
		r.commit("feat: b")
		cfg := Config{Branches: []Branch{{Name: "main"}, {Name: "next"}, {Name: "beta", Prerelease: PrereleaseFromName()}}}
		got := one(t, r.compute(Request{Config: cfg}))
		if got.NewReleaseVersion != "1.1.0-beta.1" || got.NewReleaseChannel != "beta" {
			t.Fatalf("got %q on %q, want 1.1.0-beta.1 on beta", got.NewReleaseVersion, got.NewReleaseChannel)
		}
		r.git("checkout", "-q", "next")
		r.commit("fix: n")
		got = one(t, r.compute(Request{Config: cfg}))
		if got.NewReleaseVersion != "1.0.1" || got.NewReleaseChannel != "next" {
			t.Fatalf("got %q on %q, want 1.0.1 on next", got.NewReleaseVersion, got.NewReleaseChannel)
		}
	})
	t.Run("a branch matching no entry has no release, with a reason", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: initial")
		r.git("checkout", "-q", "-b", "feature/x")
		r.commit("feat: wip")
		got := one(t, r.compute(Request{}))
		if got.NewReleasePublished || !strings.Contains(got.Reason, "feature/x") || got.Branch != "feature/x" {
			t.Fatalf("got %+v, want no release naming the branch", got)
		}
	})
	t.Run("an invalid prerelease id is refused as semantic-release does", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: initial")
		r.git("checkout", "-q", "-b", "hotfix/x")
		cfg := Config{Branches: []Branch{{Name: "main"}, {Name: "hotfix/*", Prerelease: PrereleaseFromName()}}}
		_, err := r.computeErr(Request{Config: cfg})
		if err == nil || !strings.Contains(err.Error(), "EPRERELEASEBRANCH") {
			t.Fatalf("err = %v, want EPRERELEASEBRANCH", err)
		}
	})
}

// Rule 3: tag formats and the last release.
func TestComputeTags(t *testing.T) {
	t.Run("a first release is 1.0.0", func(t *testing.T) {
		r := newTestRepo(t)
		head := r.commit("fix: first")
		got := one(t, r.compute(Request{}))
		if got.NewReleaseVersion != "1.0.0" || got.NewReleaseGitTag != "v1.0.0" || got.LastReleaseVersion != "" || got.NewReleaseGitHead != head {
			t.Fatalf("got %+v, want 1.0.0 tagged v1.0.0 at %s", got, head)
		}
	})
	t.Run("a first prerelease is 1.0.0-alpha.1", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: first")
		r.git("checkout", "-q", "-b", "develop")
		got := one(t, r.compute(Request{}))
		if got.NewReleaseVersion != "1.0.0-alpha.1" {
			t.Fatalf("got %q, want 1.0.0-alpha.1", got.NewReleaseVersion)
		}
	})
	t.Run("the highest reachable tag of the format is the last release", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: a")
		r.release("v1.0.0", "")
		r.commit("feat: b")
		r.release("v1.2.0", "")
		head := r.git("rev-parse", "HEAD")
		r.git("checkout", "-q", "-b", "side")
		r.commit("feat: unmerged")
		r.release("v9.0.0", "")
		r.git("checkout", "-q", "main")
		r.commit("fix: c")
		got := one(t, r.compute(Request{}))
		if got.LastReleaseVersion != "1.2.0" || got.LastReleaseGitTag != "v1.2.0" || got.LastReleaseGitHead != head || got.NewReleaseVersion != "1.2.1" {
			t.Fatalf("got %+v, want 1.2.1 after v1.2.0 at %s", got, head)
		}
	})
	t.Run("tag_format selects its own tags", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: a")
		r.release("v5.0.0", "")
		r.release("api-v1.0.0", "")
		r.commit("feat: b")
		got := one(t, r.compute(Request{Config: Config{TagFormat: "api-v${version}"}}))
		if got.LastReleaseGitTag != "api-v1.0.0" || got.NewReleaseVersion != "1.1.0" || got.NewReleaseGitTag != "api-v1.1.0" {
			t.Fatalf("got %+v, want api-v1.1.0 after api-v1.0.0", got)
		}
	})
	t.Run("no commit since the last release is no release, with a reason", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("feat: a")
		r.release("v1.0.0", "")
		got := one(t, r.compute(Request{}))
		if got.NewReleasePublished || got.Reason == "" || got.LastReleaseVersion != "1.0.0" {
			t.Fatalf("got %+v, want no release with a reason", got)
		}
	})
}

// Rule 4: monorepo filters, depends_on and dependency order.
func TestComputeMonorepo(t *testing.T) {
	cfg := Config{Projects: []Project{
		{Name: "web", Path: "web", DependsOn: []Dependency{{Project: "lib", Bump: ReleaseMinor}}},
		{Name: "lib", Path: "lib", SharedPaths: []string{"build/*.props"}, Scopes: []string{"lib", "core"}},
		{Name: "api", Path: "api", DependsOn: []Dependency{{Project: "lib"}}},
	}}
	t.Run("projects in dependency order, each with its own commits", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("chore: init", "web/a", "lib/a", "api/a")
		r.release("web-v1.0.0", "")
		r.release("lib-v1.0.0", "")
		r.release("api-v1.0.0", "")
		r.commit("feat: web only", "web/b")
		results := r.compute(Request{Config: cfg})
		var order []string
		for _, res := range results {
			order = append(order, res.Project)
		}
		if !slices.Equal(order, []string{"lib", "web", "api"}) {
			t.Fatalf("order = %v, want [lib web api]", order)
		}
		m := byProject(results)
		if m["web"].NewReleaseVersion != "1.1.0" || m["web"].NewReleaseGitTag != "web-v1.1.0" || m["lib"].NewReleasePublished || m["api"].NewReleasePublished {
			t.Fatalf("got %+v, want only web 1.1.0", m)
		}
		if m["web"].Changelog != "web/CHANGELOG.md" {
			t.Fatalf("web changelog = %q", m["web"].Changelog)
		}
	})
	t.Run("shared paths and scopes count; depends_on gives at least its bump", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("chore: init", "web/a", "lib/a", "api/a")
		r.release("web-v1.0.0", "")
		r.release("lib-v1.0.0", "")
		r.release("api-v1.0.0", "")
		r.commit("fix: props", "build/common.props")
		m := byProject(r.compute(Request{Config: cfg}))
		if m["lib"].NewReleaseVersion != "1.0.1" {
			t.Fatalf("lib = %q, want 1.0.1 from the shared path", m["lib"].NewReleaseVersion)
		}
		if m["web"].NewReleaseVersion != "1.1.0" || m["api"].NewReleaseVersion != "1.0.1" {
			t.Fatalf("web = %q api = %q, want 1.1.0 (minor) and 1.0.1 (default patch)", m["web"].NewReleaseVersion, m["api"].NewReleaseVersion)
		}
		if m["web"].NewReleaseNotes == "" || strings.Contains(m["web"].NewReleaseNotes, "props") {
			t.Fatalf("web notes = %q: want a release without lib's commit", m["web"].NewReleaseNotes)
		}
		r.release("lib-v1.0.1", "")
		r.release("web-v1.1.0", "")
		r.release("api-v1.0.1", "")
		r.commit("feat(core): anywhere", "docs/x")
		m = byProject(r.compute(Request{Config: cfg}))
		if m["lib"].NewReleaseVersion != "1.1.0" {
			t.Fatalf("lib = %q, want 1.1.0 from the core scope", m["lib"].NewReleaseVersion)
		}
	})
	t.Run("selected projects come with their dependencies' effect", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("chore: init", "web/a", "lib/a", "api/a")
		r.commit("fix: lib", "lib/b")
		results := r.compute(Request{Config: cfg, Projects: []string{"web"}})
		got := one(t, results)
		if got.Project != "web" || got.NewReleaseVersion != "1.0.0" {
			t.Fatalf("got %+v, want web 1.0.0", got)
		}
	})
	t.Run("an unknown selected project is an error", func(t *testing.T) {
		r := newTestRepo(t)
		r.commit("chore: init")
		if _, err := r.computeErr(Request{Config: cfg, Projects: []string{"nope"}}); err == nil {
			t.Fatal("want an error")
		}
	})
}

// Rule 5: the release notes and CHANGELOG section, as the conventionalcommits
// preset renders them.
func TestComputeNotes(t *testing.T) {
	t.Run("with a GitHub origin", func(t *testing.T) {
		r := newTestRepo(t)
		r.git("remote", "add", "origin", "https://token@github.com/acme/widget.git")
		r.commit("chore: init")
		r.release("v1.1.0", "")
		fix := r.commit("fix(api): handle nil (#12)\n\nCloses acme/other#47")
		feat := r.commit("feat: zebra support")
		feat2 := r.commit("feat(api): add alpha endpoint")
		brk := r.commit("perf(db)!: drop the cache")
		r.commit("docs: hidden")
		got := one(t, r.compute(Request{}))
		u := "https://github.com/acme/widget"
		want := "## [2.0.0](" + u + "/compare/v1.1.0...v2.0.0) (2026-10-23)\n\n" +
			"### ⚠ BREAKING CHANGES\n\n" +
			"* **db:** drop the cache\n\n" +
			"### Features\n\n" +
			"* **api:** add alpha endpoint ([" + feat2[:7] + "](" + u + "/commit/" + feat2 + "))\n" +
			"* zebra support ([" + feat[:7] + "](" + u + "/commit/" + feat + "))\n\n" +
			"### Bug Fixes\n\n" +
			"* **api:** handle nil ([#12](" + u + "/issues/12)) ([" + fix[:7] + "](" + u + "/commit/" + fix + ")), closes [acme/other#47](https://github.com/acme/other/issues/47)\n\n" +
			"### Performance Improvements\n\n" +
			"* **db:** drop the cache ([" + brk[:7] + "](" + u + "/commit/" + brk + "))\n"
		if got.NewReleaseNotes != want {
			t.Fatalf("notes:\n%s\nwant:\n%s", got.NewReleaseNotes, want)
		}
		if got.ChangelogSection != want || got.Changelog != "CHANGELOG.md" || got.Bump != ReleaseMajor {
			t.Fatalf("section %q changelog %q bump %q", got.ChangelogSection, got.Changelog, got.Bump)
		}
	})
	t.Run("without a public origin the notes have no links", func(t *testing.T) {
		r := newTestRepo(t)
		fix := r.commit("fix: first one")
		got := one(t, r.compute(Request{}))
		want := "## 1.0.0 (2026-10-23)\n\n### Bug Fixes\n\n* first one (" + fix[:7] + ")\n"
		if got.NewReleaseNotes != want {
			t.Fatalf("notes:\n%q\nwant:\n%q", got.NewReleaseNotes, want)
		}
	})
}

// Rule 6: Compute goes through the injected runner.
func TestComputeUsesTheInjectedRunner(t *testing.T) {
	fake := &envtest.FakeRunner{Default: envtest.Response{Stderr: "fatal: not a git repository", ExitCode: 128}}
	_, err := Compute(context.Background(), Request{RepoDir: "/nowhere", Runner: fake, Offline: true})
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("err = %v, want a not-a-git-repository error", err)
	}
	if len(fake.Calls) == 0 || fake.Calls[0].Name != "git" {
		t.Fatalf("calls = %+v, want git through the runner", fake.Calls)
	}
}

// Rule 6: the result's outputs.
func TestResultOutputs(t *testing.T) {
	r := newTestRepo(t)
	r.commit("feat: a")
	r.release("v1.4.2", "")
	r.git("checkout", "-q", "-b", "develop")
	head := r.commit("feat: b")
	got := one(t, r.compute(Request{}))
	want := map[string]string{
		"new_release_published":     "true",
		"new_release_version":       "1.5.0-alpha.1",
		"new_release_major_version": "1",
		"new_release_minor_version": "5",
		"new_release_patch_version": "0",
		"new_release_channel":       "alpha",
		"new_release_git_head":      head,
		"new_release_git_tag":       "v1.5.0-alpha.1",
		"last_release_version":      "1.4.2",
		"last_release_git_tag":      "v1.4.2",
	}
	outs := got.Outputs()
	if len(outs) != 12 {
		t.Fatalf("%d outputs, want 12", len(outs))
	}
	for _, o := range outs {
		if w, ok := want[o.Name]; ok && o.Value != w {
			t.Errorf("%s = %q, want %q", o.Name, o.Value, w)
		}
	}
	if got.Project == "" || got.Branch != "develop" || got.Bump != ReleaseMinor || got.LastReleaseGitHead == "" || got.NewReleaseNotes == "" {
		t.Fatalf("got %+v", got)
	}
}

// Outside offline mode, origin's release tags and semantic-release notes are
// fetched first, as semantic-release does.
func TestComputeFetchesOriginReleases(t *testing.T) {
	origin := newTestRepo(t)
	origin.commit("feat: initial")
	origin.release("v1.0.0", "")
	origin.git("checkout", "-q", "-b", "develop")
	origin.commit("feat: one")
	origin.release("v1.1.0-alpha.1", "alpha")

	r := newTestRepo(t)
	r.git("remote", "add", "origin", origin.dir)
	r.git("fetch", "-q", "--no-tags", "origin", "+refs/heads/*:refs/remotes/origin/*")
	r.git("checkout", "-q", "-b", "develop", "origin/develop")
	r.commit("fix: two")

	offline := one(t, r.compute(Request{}))
	if offline.NewReleaseVersion != "1.0.0-alpha.1" {
		t.Fatalf("offline = %q, want 1.0.0-alpha.1 (no tags locally)", offline.NewReleaseVersion)
	}
	results, err := Compute(context.Background(), Request{RepoDir: r.dir, Runner: env.ExecRunner{}, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	got := one(t, results)
	if got.LastReleaseVersion != "1.1.0-alpha.1" || got.NewReleaseVersion != "1.1.0-alpha.2" {
		t.Fatalf("got %q after %q, want 1.1.0-alpha.2 after origin's 1.1.0-alpha.1", got.NewReleaseVersion, got.LastReleaseVersion)
	}
}
