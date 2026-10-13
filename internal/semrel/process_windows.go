//go:build windows

package semrel

import (
	"os/exec"
	"strconv"
	"syscall"
)

// killGroupOnCancel starts cmd in a new process group and, when its context
// is cancelled, kills its whole process tree with taskkill /T, falling back
// to killing the direct child when taskkill fails.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error {
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
