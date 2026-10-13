//go:build !windows

package semrel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestExecRunner_CancelKillsTheWholeProcessGroup runs a real shell whose
// background grandchild keeps stdout open, like an ssh or
// git-remote-https helper under git fetch. Cancelling the context must kill
// the grandchild too and return within the wait delay; killing only the
// direct child would leave Wait blocked on the open pipe.
func TestExecRunner_CancelKillsTheWholeProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	readPid := func() int {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return 0
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		return pid
	}
	// Kill the leftovers even when the test fails (also unblocks a hung Run).
	t.Cleanup(func() {
		if pid := readPid(); pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := ExecRunner{}.Run(ctx, Command{Name: "sh", Args: []string{"-c", `sleep 60 & echo $! > "$0"; sleep 60`, pidFile}})
		done <- err
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(processWaitDelay + 3*time.Second):
		t.Fatalf("Run still blocked %s after the context deadline: a grandchild holds the output pipe", time.Since(start))
	}
	if err == nil {
		t.Fatal("Run returned no error for a cancelled command")
	}
	if elapsed := time.Since(start); elapsed > processWaitDelay+2*time.Second {
		t.Errorf("Run returned after %s, want within the wait delay %s", elapsed, processWaitDelay)
	}

	pid := readPid()
	if pid <= 0 {
		t.Fatal("the shell never started its background grandchild")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d survived the cancellation", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
