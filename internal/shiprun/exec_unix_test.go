//go:build !windows

package shiprun

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// A timed-out script dies with the child it started: the whole process
// tree is killed, not only bash.
func TestReal_TimeoutKillsTheScriptsChildren(t *testing.T) {
	needShell(t, "bash")
	root := realRepo(t, nil)
	pidFile := filepath.Join(root, "child.pid")
	readPid := func() int {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return 0
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		return pid
	}
	t.Cleanup(func() {
		if pid := readPid(); pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	start := time.Now()
	_, err := runReal(t, root, "test", shipconfig.Command{Run: `sleep 60 & echo $! > child.pid; wait`, Shell: "bash", Timeout: "300ms"}, Vars{})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Errorf("returned after %s", elapsed)
	}
	pid := readPid()
	if pid <= 0 {
		t.Fatal("the script never started its child")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("child %d survived the timeout", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
