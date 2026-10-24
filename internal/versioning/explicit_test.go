package versioning

import (
	"strings"
	"testing"
)

// L21 (ship --version): an explicit version is released as given, with the
// last release picked as the engine picks it (tags reachable from the
// branch, of its channel) and the notes and CHANGELOG section written for
// it from the commits since that release.
func TestComputeExplicitVersion(t *testing.T) {
	r := newTestRepo(t)
	r.commit("feat: initial")
	r.git("checkout", "-q", "-b", "develop")
	r.commit("feat: first alpha")
	r.release("v1.0.0-alpha.1", "alpha")
	r.git("checkout", "-q", "main")
	r.commit("fix: only on main")
	r.release("v1.0.0", "") // stable, never merged into develop
	r.git("checkout", "-q", "develop")
	r.commit("feat: add gadgets")
	r.commit("docs: explain gadgets")

	computed := one(t, r.compute(Request{}))
	if computed.NewReleaseVersion != "1.0.0-alpha.2" {
		t.Fatalf("computed %q, want 1.0.0-alpha.2", computed.NewReleaseVersion)
	}
	got := one(t, r.compute(Request{Version: computed.NewReleaseVersion}))
	if !got.NewReleasePublished || got.NewReleaseVersion != "1.0.0-alpha.2" || got.NewReleaseGitTag != "v1.0.0-alpha.2" || got.NewReleaseChannel != "alpha" {
		t.Fatalf("explicit result %+v", got)
	}
	if got.LastReleaseVersion != "1.0.0-alpha.1" || got.LastReleaseGitTag != "v1.0.0-alpha.1" {
		t.Errorf("last release %q (%q), want 1.0.0-alpha.1", got.LastReleaseVersion, got.LastReleaseGitTag)
	}
	if got.NewReleaseNotes != computed.NewReleaseNotes || got.ChangelogSection != computed.ChangelogSection {
		t.Errorf("notes differ from the computed release's:\n%s\nwant\n%s", got.NewReleaseNotes, computed.NewReleaseNotes)
	}
	if !strings.Contains(got.NewReleaseNotes, "add gadgets") || strings.Contains(got.NewReleaseNotes, "only on main") {
		t.Errorf("notes:\n%s", got.NewReleaseNotes)
	}

	// Another version than the computed one: its own heading, the same
	// commits; no triggering commit still releases it.
	other := one(t, r.compute(Request{Version: "1.1.0-alpha.1"}))
	if !strings.Contains(other.ChangelogSection, "1.1.0-alpha.1") || !strings.Contains(other.NewReleaseNotes, "add gadgets") {
		t.Errorf("1.1.0-alpha.1 notes:\n%s", other.NewReleaseNotes)
	}
	r.commit("feat: more")
	r.release("v1.0.0-alpha.2", "alpha")
	r.commit("docs: only docs")
	quiet := one(t, r.compute(Request{Version: "1.0.0-alpha.3"}))
	if !quiet.NewReleasePublished || quiet.NewReleaseVersion != "1.0.0-alpha.3" || quiet.LastReleaseVersion != "1.0.0-alpha.2" {
		t.Errorf("no triggering commit: %+v", quiet)
	}

	// A tag that already exists, even on another branch, is refused.
	if _, err := r.computeErr(Request{Version: "1.0.0"}); err == nil || !strings.Contains(err.Error(), "v1.0.0 already exists") {
		t.Errorf("existing tag: err %v", err)
	}
	// Not a version.
	if _, err := r.computeErr(Request{Version: "1.0"}); err == nil {
		t.Error("an invalid version was accepted")
	}
}
