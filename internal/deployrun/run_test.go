package deployrun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

const tag = "v1.0.0-alpha.1"

// A deploy makes the release commit on the base commit, the lightweight tag,
// semantic-release's channel note, pushes all three, publishes the
// artifact's images, creates the GitHub prerelease, then runs the target's
// publish, deploy and verify.
func TestRun_ReleasesAndDeploysTheArtifact(t *testing.T) {
	e := newEnv(t)
	rep, err := e.run(e.options())
	if err != nil {
		t.Fatal(err)
	}
	wantStatuses(t, rep, map[string]string{
		"verify-artifact": StatusDone, "clone": StatusDone, "commit": StatusDone, "tag": StatusDone, "notes": StatusDone,
		"push": StatusDone, "publish": StatusDone, "release": StatusDone,
		"publish:staging": StatusDone, "deploy:staging": StatusDone, "verify:staging": StatusDone,
	})

	head := git(t, e.origin, "rev-parse", "refs/heads/develop")
	if msg := git(t, e.origin, "log", "-1", "--format=%B", head); msg != strings.TrimSpace("chore(release): 1.0.0-alpha.1 [skip ci]\n\n"+notes) {
		t.Errorf("release commit message %q", msg)
	}
	if parent := git(t, e.origin, "rev-parse", head+"^"); parent != e.base {
		t.Errorf("release commit parent %s, want the base %s", parent, e.base)
	}
	if got := git(t, e.origin, "show", head+":CHANGELOG.md"); got != strings.TrimSpace(changelog) {
		t.Errorf("CHANGELOG.md at the release commit %q", got)
	}
	// A lightweight tag, as semantic-release makes it: its channel note on
	// the release commit is also the tag's note for semantic-release's
	// "git notes show <tag>", so both engines read it.
	if kind := git(t, e.origin, "cat-file", "-t", "refs/tags/"+tag); kind != "commit" {
		t.Errorf("tag is a %s, want a lightweight tag on the release commit", kind)
	}
	if peeled := git(t, e.origin, "rev-parse", tag+"^{commit}"); peeled != head {
		t.Errorf("tag points at %s, want the release commit %s", peeled, head)
	}
	if note := git(t, e.origin, "notes", "--ref", "semantic-release-"+tag, "show", head); note != `{"channels":["alpha"]}` {
		t.Errorf("channel note %q", note)
	}

	calls := e.log()
	tar := filepath.Join(e.dir, "outputs", "app.tar")
	want := []string{
		preflightCall,
		"docker [load] [-i] [" + tar + "]",
		"docker [push] [reg.example/app:1.0.0-alpha.1]",
		"gh [release] [view] [" + tag + "]",
		"gh [release] [create] [" + tag + "] [--title] [" + tag + "] [--notes-file] [" + filepath.Join(e.dir, "notes.md") + "] [--prerelease] [--verify-tag]",
		"docker [load] [-i] [" + tar + "]",
		"docker [push] [reg.example/app:1.0.0-alpha.1]",
		"nerv [provider] [deploy-ssh-compose] [deploy]",
		"nerv [provider] [deploy-ssh-compose] [verify]",
	}
	if !slices.Equal(calls, want) {
		t.Errorf("provider calls:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
	stdin, _ := os.ReadFile(filepath.Join(e.fake, "nerv.stdin"))
	for _, s := range []string{`"contract":1`, `"extension":"deploy"`, `"slot":"target"`, `"provider":"ssh-compose"`, `"operation":"verify"`,
		`"payload":{"target":"staging"`, `"environment":"staging"`, `"ssh_port":"22"`, `"version":"1.0.0-alpha.1"`} {
		if !strings.Contains(string(stdin), s) {
			t.Errorf("verify request lacks %s: %s", s, stdin)
		}
	}
}

// The channel note is the one semantic-release writes: after the deploy,
// the version engine on origin numbers the next prerelease of the channel.
func TestRun_NextPrereleaseIsNumberedFromTheDeployedTag(t *testing.T) {
	e := newEnv(t)
	if _, err := e.run(e.options()); err != nil {
		t.Fatal(err)
	}
	e.pushOther("app.txt", "two\n")
	clone := filepath.Join(t.TempDir(), "clone")
	git(t, filepath.Dir(clone), "clone", "-q", "--branch", "develop", e.origin, clone)
	res, err := versioning.Compute(context.Background(), versioning.Request{RepoDir: clone, Branch: "develop", Runner: env.ExecRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].NewReleaseVersion != "1.0.0-alpha.2" || res[0].LastReleaseGitTag != tag {
		t.Errorf("next release %+v, want 1.0.0-alpha.2 after %s", res, tag)
	}
}

// A second deploy of the same artifact changes nothing on origin and skips
// every step already done, including the existing GitHub release.
func TestRun_IsIdempotent(t *testing.T) {
	e := newEnv(t)
	if _, err := e.run(e.options()); err != nil {
		t.Fatal(err)
	}
	refs := e.originRefs()
	rep, err := e.run(e.options())
	if err != nil {
		t.Fatal(err)
	}
	wantStatuses(t, rep, map[string]string{
		"commit": StatusSkipped, "tag": StatusSkipped, "notes": StatusSkipped, "push": StatusSkipped, "release": StatusSkipped,
	})
	if after := e.originRefs(); after != refs {
		t.Errorf("origin changed on a re-run:\n%s\nwas:\n%s", after, refs)
	}
	creates := 0
	for _, c := range e.log() {
		if strings.HasPrefix(c, "gh [release] [create]") {
			creates++
		}
	}
	if creates != 1 {
		t.Errorf("the release was created %d times", creates)
	}
}

// A failed publish reports exactly what was done; the re-run resumes: the
// git steps are skipped and the rest runs.
func TestRun_ReportsAPartialFailureAndResumes(t *testing.T) {
	e := newEnv(t)
	writeFile(t, filepath.Join(e.fake, "fail-push"), "", 0o644)
	rep, err := e.run(e.options())
	if err == nil || !strings.Contains(err.Error(), "denied: push access") {
		t.Fatalf("Run: %v, want the docker push failure", err)
	}
	wantStatuses(t, rep, map[string]string{
		"commit": StatusDone, "tag": StatusDone, "notes": StatusDone, "push": StatusDone,
		"publish": StatusFailed, "release": StatusNotRun,
		"publish:staging": StatusNotRun, "deploy:staging": StatusNotRun, "verify:staging": StatusNotRun,
	})
	refs := e.originRefs()

	os.Remove(filepath.Join(e.fake, "fail-push"))
	rep, err = e.run(e.options())
	if err != nil {
		t.Fatal(err)
	}
	wantStatuses(t, rep, map[string]string{
		"commit": StatusSkipped, "tag": StatusSkipped, "push": StatusSkipped,
		"publish": StatusDone, "release": StatusDone,
		"publish:staging": StatusDone, "deploy:staging": StatusDone, "verify:staging": StatusDone,
	})
	if after := e.originRefs(); after != refs {
		t.Errorf("the resume changed origin's refs")
	}
}

// When origin's branch moved past the base commit, deploy refuses unless
// --rebase, which re-applies the release files on the new head.
func TestRun_OriginMovedNeedsRebase(t *testing.T) {
	e := newEnv(t)
	e.pushOther("other.txt", "x\n")
	moved := e.originRefs()
	_, err := e.run(e.options())
	if !refusal.Is(err) || !strings.Contains(err.Error(), "--rebase") {
		t.Fatalf("Run: %v, want a refusal naming --rebase", err)
	}
	if e.originRefs() != moved || e.effects()[0] != "" {
		t.Errorf("a refused deploy touched origin or ran a provider: %v", e.log())
	}

	opts := e.options()
	opts.Rebase = true
	if _, err := e.run(opts); err != nil {
		t.Fatal(err)
	}
	head := git(t, e.origin, "rev-parse", "refs/heads/develop")
	newBase := strings.Fields(moved)[1]
	if parent := git(t, e.origin, "rev-parse", head+"^"); !strings.Contains(moved, parent) || parent == e.base {
		t.Errorf("rebased release commit's parent %s, want origin's new head (%s)", parent, newBase)
	}
	if got := git(t, e.origin, "show", head+":other.txt"); got != "x" {
		t.Errorf("the rebased release lost origin's new commit")
	}
	if got := git(t, e.origin, "show", head+":CHANGELOG.md"); got != strings.TrimSpace(changelog) {
		t.Errorf("CHANGELOG.md after the rebase %q", got)
	}
}

func TestRun_RebaseRefusesAConflict(t *testing.T) {
	e := newEnv(t)
	e.pushOther("CHANGELOG.md", "# Changelog\n\nsomeone else\n")
	moved := e.originRefs()
	opts := e.options()
	opts.Rebase = true
	_, err := e.run(opts)
	if !refusal.Is(err) || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("Run: %v, want a conflict refusal", err)
	}
	if e.originRefs() != moved {
		t.Errorf("a refused rebase touched origin")
	}
}

// A dry run prepares the release in the clone but pushes nothing and runs
// no provider.
func TestRun_DryRunWritesNothing(t *testing.T) {
	e := newEnv(t)
	refs := e.originRefs()
	opts := e.options()
	opts.DryRun = true
	rep, err := e.run(opts)
	if err != nil {
		t.Fatal(err)
	}
	wantStatuses(t, rep, map[string]string{
		"commit": StatusDone, "push": StatusDryRun, "publish": StatusDryRun, "publish:staging": StatusDryRun, "release": StatusDryRun,
	})
	if e.originRefs() != refs || e.effects()[0] != "" || e.log()[0] != preflightCall {
		t.Errorf("a dry run touched origin or ran a provider (only the preflight): %v", e.log())
	}
}

// Confirm runs after the release is prepared and before anything is
// pushed; declining leaves origin as it was.
func TestRun_ConfirmationGuardsThePush(t *testing.T) {
	e := newEnv(t)
	refs := e.originRefs()
	opts := e.options()
	asked := false
	opts.Confirm = func(Report) error {
		asked = true
		return &refusal.Error{Err: os.ErrPermission}
	}
	if _, err := e.run(opts); !refusal.Is(err) || !asked {
		t.Fatalf("Run: %v (asked %v)", err, asked)
	}
	if e.originRefs() != refs {
		t.Errorf("a declined deploy pushed")
	}
}

func TestRun_RefusesATagOnOriginWithOtherContent(t *testing.T) {
	e := newEnv(t)
	git(t, e.repo, "tag", tag, e.base)
	git(t, e.repo, "push", "-q", "origin", "refs/tags/"+tag)
	_, err := e.run(e.options())
	if !refusal.Is(err) || !strings.Contains(err.Error(), "already exists on origin") {
		t.Fatalf("Run: %v", err)
	}
}

func TestRun_RefusesATamperedArtifactBeforeCloning(t *testing.T) {
	e := newEnv(t)
	writeFile(t, filepath.Join(e.dir, "notes.md"), "rewritten", 0o644)
	rep, err := e.run(e.options())
	if !refusal.Is(err) || !strings.Contains(err.Error(), "notes.md") {
		t.Fatalf("Run: %v", err)
	}
	wantStatuses(t, rep, map[string]string{"verify-artifact": StatusFailed, "clone": StatusNotRun})
}

func TestRun_RefusesInvalidTargetConfigAndMissingTools(t *testing.T) {
	e := newEnv(t)
	e.cfg.Targets[0].Config["ssh_prot"] = "22"
	if _, err := e.run(e.options()); !refusal.Is(err) || !strings.Contains(err.Error(), "ssh_prot") {
		t.Errorf("unknown key: %v", err)
	}
	delete(e.cfg.Targets[0].Config, "ssh_prot")
	e.cfg.Targets[0].Config["ssh_port"] = "twenty"
	if _, err := e.run(e.options()); !refusal.Is(err) || !strings.Contains(err.Error(), "ssh_port") {
		t.Errorf("bad int: %v", err)
	}
	e.cfg.Targets[0].Config["ssh_port"] = "22"
	e.cfg.Host = "gitlab"
	if _, err := e.run(e.options()); !refusal.Is(err) || !strings.Contains(err.Error(), "gitlab") {
		t.Errorf("unknown provider: %v", err)
	}
	e.cfg.Host = "github"
	opts := e.options()
	opts.LookPath = func(name string) (string, error) {
		if name == "gh" {
			return "", exec.ErrNotFound
		}
		return exec.LookPath(name)
	}
	if _, err := e.run(opts); !refusal.Is(err) || !strings.Contains(err.Error(), "gh is not on PATH") {
		t.Errorf("missing gh: %v", err)
	}
	opts = e.options()
	opts.Targets = []string{"prod"}
	if _, err := e.run(opts); !refusal.Is(err) || !strings.Contains(err.Error(), "prod") {
		t.Errorf("unknown --target: %v", err)
	}
}
