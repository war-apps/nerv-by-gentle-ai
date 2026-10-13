//go:build !windows

package semrel

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// killGroupOnCancel starts cmd as the leader of a new process group and,
// when its context is cancelled, sends SIGKILL to the whole group instead
// of the direct child only.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// With Setpgid and no Pgid the group id is the child's pid.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
