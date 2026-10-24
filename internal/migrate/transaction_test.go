package migrate_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// An Apply that fails part way rolls back everything it did: the
// repository is byte-identical to before (no new file, no backup, no
// .nerv/.gitignore), and a second run then migrates it.
func TestInterrupted_ApplyRollsBackEverything(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	w := newWorld(t, comprobantes)
	w.openspecFixture(true)
	before := treeHash(t, w.root)
	odd := filepath.Join(w.root, "odd")
	if err := os.MkdirAll(filepath.Join(odd, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(odd, 0o555); err != nil {
		t.Fatal(err)
	}
	_, err := migrate.Apply(w.plan(w.options(false, githubOrigin)), now)
	if cerr := os.Chmod(odd, 0o755); cerr != nil {
		t.Fatal(cerr)
	}
	if err == nil {
		t.Fatal("Apply succeeded with a read-only odd/")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Errorf("error = %v, want it to say the run was rolled back", err)
	}
	if err := os.RemoveAll(odd); err != nil {
		t.Fatal(err)
	}
	if got := treeHash(t, w.root); got != before {
		t.Error("the interrupted Apply left changes behind")
	}
	if w.exists(".nerv/backups") || w.exists(".nerv/.gitignore") {
		t.Error("the rollback left its backups or .nerv/.gitignore")
	}
	w.migrate(w.options(false, githubOrigin))
	if !w.exists("odd/tasks/tw-1-feature.md") || w.exists("openspec/changes/tw-1-feature") {
		t.Error("the run after the rollback did not migrate the change")
	}
}

// A run killed after it wrote its new files (no rollback could run)
// leaves its own ODD document and .ai-workflow.yml; the next run
// recognises them and finishes the moves.
func TestInterrupted_RerunFinishesItsOwnHalfWrittenOutput(t *testing.T) {
	w := newWorld(t, comprobantes)
	w.openspecFixture(true)
	p := w.plan(w.options(false, githubOrigin))
	if _, err := paths.EnsureRuntimeIgnored(w.root); err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Changes {
		if c.Kind == migrate.Write {
			w.write(c.Path, string(c.New))
		}
	}
	doc := w.read("odd/tasks/tw-1-feature.md")
	// A later day: the document's date would differ, it must be kept.
	p2, err := migrate.Compute(migrate.Options{RepoRoot: w.root, Home: w.home, ExtensionsFS: w.options(false, "").ExtensionsFS, OriginURL: githubOrigin, Now: now.Add(48 * time.Hour)})
	if err != nil {
		t.Fatalf("the rerun refuses its own half-written output: %v", err)
	}
	if _, err := migrate.Apply(p2, now.Add(48*time.Hour)); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if got := w.read("odd/tasks/tw-1-feature.md"); got != doc {
		t.Errorf("the rerun rewrote the document:\n%s", got)
	}
	for _, rel := range []string{"odd/tw-1-feature/nerv/state.yaml", ".nerv/locks/tw-1-feature.lock", ".nerv/nerv.yaml.migrated"} {
		if !w.exists(rel) {
			t.Errorf("%s is missing after the rerun", rel)
		}
	}
	if w.exists("openspec/changes/tw-1-feature") || w.exists(".nerv/nerv.yaml") {
		t.Error("the rerun left old files behind")
	}
}

// A document at odd/tasks/<change>.md that nerv migrate did not write is
// still refused.
func TestODD_ForeignDocumentIsStillRefused(t *testing.T) {
	w := newWorld(t, comprobantes)
	w.openspecFixture(true)
	w.write("odd/tasks/tw-1-feature.md", "# tw-1-feature\n\nhand written\n")
	_, err := migrate.Compute(w.options(false, githubOrigin))
	if !refusal.Is(err) || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Compute = %v, want the already-exists refusal", err)
	}
}

const testHost = "builder-7"

func lockAt(heartbeat time.Time, host, waiting string) string {
	return "session_id: 0b6f-live\nhost: " + host + "\nstarted_at: \"2026-10-23T10:00:00Z\"\nheartbeat_at: \"" + heartbeat.UTC().Format(time.RFC3339) + "\"\nphase: 2\nstep: 3\nwaiting_on: " + waiting + "\npid: null\n"
}

// A change whose lock is held on this host (a heartbeat younger than the
// orchestrator's 15-minute stale threshold, or a run waiting on the user)
// is refused, naming the session and the heartbeat; --force moves it.
func TestLock_LiveLockIsRefusedUnlessForced(t *testing.T) {
	const lock = "openspec/changes/tw-1-feature/nerv/.orchestrator.lock"
	for name, tc := range map[string]struct {
		content string
		refused bool
	}{
		"fresh":              {lockAt(now.Add(-5*time.Minute), testHost, "agent"), true},
		"waiting on user":    {lockAt(now.Add(-3*time.Hour), testHost, "user"), true},
		"stale":              {lockAt(now.Add(-20*time.Minute), testHost, "none"), false},
		"fresh, other host":  {lockAt(now.Add(-5*time.Minute), "laptop-2", "agent"), false},
		"no heartbeat (old)": {"session_id: abc\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t, comprobantes)
			w.openspecFixture(true)
			w.write(lock, tc.content)
			o := w.options(false, githubOrigin)
			o.Host = testHost
			_, err := migrate.Compute(o)
			if !tc.refused {
				if err != nil {
					t.Fatalf("Compute = %v, want no refusal", err)
				}
				return
			}
			if !refusal.Is(err) {
				t.Fatalf("Compute = %v, want a refusal", err)
			}
			hb := strings.Split(strings.Split(tc.content, "heartbeat_at: \"")[1], "\"")[0]
			contains(t, "the refusal", err.Error(), "0b6f-live", hb, "--force")
			o.Force = true
			p, err := migrate.Compute(o)
			if err != nil {
				t.Fatalf("Compute with Force = %v", err)
			}
			if _, err := migrate.Apply(p, now); err != nil {
				t.Fatal(err)
			}
			if w.read(".nerv/locks/tw-1-feature.lock") != tc.content {
				t.Error("--force did not move the lock")
			}
		})
	}
}

// L20 "Solo FULL": a DIRECT or LIGHT change moves whole to
// .nerv/runs/<change>/ (its lock to .nerv/locks/<change>.lock) and gets no
// ODD document; only a FULL change becomes odd/tasks/<change>.md.
func TestODD_DirectAndLightRunsMoveToTheRuntimeFolder(t *testing.T) {
	for _, tier := range []string{"direct", "light"} {
		t.Run(tier, func(t *testing.T) {
			w := newWorld(t, comprobantes)
			w.openspecFixture(true)
			c := "openspec/changes/tw-1-feature/"
			w.write(c+"state.yaml", "dependsOn: []\nnerv:\n  path: "+tier+"\n  branch: feature/tw-1-feature\n")
			p, _ := w.migrate(w.options(false, githubOrigin))
			if w.exists("odd/tasks/tw-1-feature.md") || w.exists("odd") {
				t.Errorf("a %s change got an ODD document or an odd/ folder", tier)
			}
			for rel, want := range map[string]string{
				".nerv/runs/tw-1-feature/state.yaml":             "dependsOn: []\nnerv:\n  path: " + tier,
				".nerv/runs/tw-1-feature/proposal.md":            "# Proposal: tw-1-feature",
				".nerv/runs/tw-1-feature/design.md":              "# Design",
				".nerv/runs/tw-1-feature/tasks.md":               "# Tasks",
				".nerv/runs/tw-1-feature/specs/receipts/spec.md": "# Receipts",
				".nerv/runs/tw-1-feature/exploration.md":         "explored\n",
				".nerv/runs/tw-1-feature/deliberation-log.md":    "log\n",
				".nerv/runs/tw-1-feature/test-plan.md":           "plan\n",
				".nerv/locks/tw-1-feature.lock":                  "session_id: abc\n",
			} {
				if !w.exists(rel) || !strings.HasPrefix(w.read(rel), want) {
					t.Errorf("%s is missing or wrong", rel)
				}
			}
			if w.exists("openspec/changes/tw-1-feature") {
				t.Error("the change folder is still there")
			}
			if !anyContains(p.Notes, "no ODD document") {
				t.Errorf("no note says the %s change gets no ODD document: %q", tier, p.Notes)
			}
			if p2 := w.plan(w.options(false, githubOrigin)); !p2.Empty() {
				t.Errorf("the second plan has changes:\n%s", migrate.Render(p2))
			}
		})
	}
}

// A repository backup keeps the source file's permission bits.
func TestBackups_KeepTheSourceFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	w := newWorld(t, comprobantes)
	w.openspecFixture(true)
	for _, rel := range []string{".nerv/nerv.yaml", "openspec/changes/tw-1-feature/tasks.md"} {
		if err := os.Chmod(filepath.Join(w.root, filepath.FromSlash(rel)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w.migrate(w.options(false, githubOrigin))
	dir := filepath.Join(w.root, ".nerv", "backups", "migrate-"+now.Format("20060102-150405"))
	for _, rel := range []string{".nerv/nerv.yaml", "openspec/changes/tw-1-feature/tasks.md", "openspec/changes/tw-1-feature/proposal.md"} {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if strings.HasSuffix(rel, "proposal.md") {
			want = 0o644
		}
		if info.Mode().Perm() != want {
			t.Errorf("backup of %s has mode %v, want %v", rel, info.Mode().Perm(), want)
		}
	}
}
