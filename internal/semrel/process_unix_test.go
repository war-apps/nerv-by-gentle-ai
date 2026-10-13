//go:build !windows

package semrel

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

// waitGone fails t unless pid disappears within two seconds.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d is still alive", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestExecRunner_LingeringDescendantKeepsASuccessfulExit runs a real shell
// that exits 0 while a background grandchild still holds stdout, like a
// helper an install script left behind. Run must report the exit as a
// success with the output written so far, not as exec.ErrWaitDelay, and
// must kill the grandchild it gave up waiting for.
func TestExecRunner_LingeringDescendantKeepsASuccessfulExit(t *testing.T) {
	saved := processWaitDelay
	processWaitDelay = 300 * time.Millisecond
	t.Cleanup(func() { processWaitDelay = saved })

	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
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
	res, err := ExecRunner{}.Run(context.Background(), Command{Name: "sh", Args: []string{"-c", `sleep 30 & echo $! > "$0"; echo ok`, pidFile}})
	if err != nil {
		t.Fatalf("Run(exit 0 with a lingering grandchild) = %v, want success", err)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "ok" {
		t.Errorf("stdout = %q, want %q", got, "ok")
	}
	if elapsed := time.Since(start); elapsed > processWaitDelay+2*time.Second {
		t.Errorf("Run returned after %s, want about the wait delay %s", elapsed, processWaitDelay)
	}
	pid := readPid()
	if pid <= 0 {
		t.Fatal("the shell never started its background grandchild")
	}
	waitGone(t, pid)
}

// TestExecRunner_ChildHasNoControllingTerminal runs a real shell that reads
// from /dev/tty, like an ssh passphrase or host-key prompt or a git https
// credential prompt. The command must fail at once instead of being stopped
// by SIGTTIN until the timeout: it runs in its own session, with no
// controlling terminal. The difference only shows when the test process
// itself has a controlling terminal (run it under `script` otherwise).
func TestExecRunner_ChildHasNoControllingTerminal(t *testing.T) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		t.Skipf("the test process has no controlling terminal: %v", err)
	}
	_ = tty.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	start := time.Now()
	_, err = ExecRunner{}.Run(ctx, Command{Name: "sh", Args: []string{"-c", `read line < /dev/tty`}})
	elapsed := time.Since(start)
	if ctx.Err() != nil {
		t.Fatalf("Run hit the timeout after %s: the child was stopped waiting for the terminal (err %v)", elapsed, err)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run = %v, want the shell's failure to open /dev/tty", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run returned after %s, want at once", elapsed)
	}
}
