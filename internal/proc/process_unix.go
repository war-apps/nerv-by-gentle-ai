//go:build !windows

package proc

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// killGroupOnCancel starts cmd as the leader of a new session and, when its
// context is cancelled, sends SIGKILL to its whole process group instead of
// the direct child only.
//
// A new session (Setsid) rather than only a new process group: the command
// then has no controlling terminal, so a helper that opens /dev/tty (an ssh
// passphrase or host-key prompt, a git https credential prompt) fails at
// once with ENXIO. In a background process group of the caller's session it
// would instead be stopped by SIGTTIN/SIGTTOU until the timeout. A session
// leader is also the leader of a process group whose id is its pid, so the
// group kill still reaches every descendant that did not leave the group.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return killGroup(cmd) }
}

// killLeftovers kills what remains of cmd's process group after cmd exited
// and Run stopped waiting for a descendant that held the output pipes. The
// group id cannot be reused while any member is alive, so this reaches only
// those leftovers, or nothing.
func killLeftovers(cmd *exec.Cmd) { _ = killGroup(cmd) }

// killGroup sends SIGKILL to cmd's process group, whose id is cmd's pid.
func killGroup(cmd *exec.Cmd) error {
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
