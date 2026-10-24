package deployrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// A crafted artifact must never write into the deploy clone's git
// directory: a .git/config there runs core.fsmonitor on the next git call.
func TestRun_RefusesAReleaseFileInsideDotGit(t *testing.T) {
	for _, p := range []string{".git/config", "sub/.git/config", ".GIT/config", ".git/hooks/post-commit"} {
		t.Run(p, func(t *testing.T) {
			e := newEnv(t)
			marker := filepath.Join(t.TempDir(), "pwned")
			cfg := "[core]\n\trepositoryformatversion = 0\n\tfsmonitor = \"touch " + marker + "; false\"\n[remote \"origin\"]\n\turl = " + e.origin + "\n"
			stored := filepath.Join(e.dir, "release-files", "CHANGELOG.md")
			writeFile(t, stored, cfg, 0o644)
			sum := sha256.Sum256([]byte(cfg))
			mpath := filepath.Join(e.dir, artifact.ManifestFile)
			data, _ := os.ReadFile(mpath)
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatal(err)
			}
			m["release_files"] = []map[string]string{{"path": p, "stored": "release-files/CHANGELOG.md", "sha256": hex.EncodeToString(sum[:])}}
			data, _ = json.Marshal(m)
			writeFile(t, mpath, string(data), 0o644)
			o := e.options()
			o.DryRun = true
			_, err := e.run(o)
			if _, serr := os.Stat(marker); serr == nil {
				t.Fatalf("the crafted .git/config ran a command")
			}
			if err == nil || !refusal.Is(err) || !strings.Contains(err.Error(), ".git") {
				t.Errorf("Run() = %v, want a refusal naming .git", err)
			}
		})
	}
}
