package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A change whose lock is held on this host is refused (exit 1, naming the
// session and the heartbeat); --force moves it.
func TestMigrate_LiveLockIsRefusedUnlessForce(t *testing.T) {
	top, opts := migrateRepo(t, "dim.comprobantes.api")
	host, err := os.Hostname()
	if err != nil {
		t.Skip("no hostname")
	}
	heartbeat := opts.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	lock := "session_id: 7c1e-live\nhost: " + host + "\nheartbeat_at: \"" + heartbeat + "\"\nwaiting_on: agent\n"
	change := filepath.Join(top, "openspec", "changes", "tw-9-live")
	for rel, content := range map[string]string{
		"state.yaml":              "nerv:\n  path: light\n",
		"nerv/.orchestrator.lock": lock,
	} {
		path := filepath.Join(change, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errOut := runMigrateCLI(t, opts)
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s\n%s", code, out, errOut)
	}
	for _, want := range []string{"7c1e-live", heartbeat, "--force"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errOut)
		}
	}
	if code, out, errOut := runMigrateCLI(t, opts, "--force"); code != 0 {
		t.Fatalf("--force: exit %d\n%s\n%s", code, out, errOut)
	}
	data, err := os.ReadFile(filepath.Join(top, ".nerv", "locks", "tw-9-live.lock"))
	if err != nil || string(data) != lock {
		t.Errorf("--force did not move the lock: %v", err)
	}
}
