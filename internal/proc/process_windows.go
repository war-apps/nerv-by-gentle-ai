//go:build windows

package proc

import (
	"context"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// taskkillTimeout bounds the taskkill call that kills a cancelled command's
// process tree, so a hung taskkill cannot keep the cancellation waiting.
const taskkillTimeout = 10 * time.Second

// killGroupOnCancel starts cmd in a new process group and, when its context
// is cancelled, kills its whole process tree with taskkill /T, falling back
// to killing the direct child when taskkill fails or times out.
//
// CREATE_NEW_PROCESS_GROUP also means a console Ctrl-C (and Ctrl-Break) no
// longer reaches the command or its children: the CLI must cancel the
// context on SIGINT/SIGTERM (signal.NotifyContext) for a Ctrl-C to stop
// git, npm and node.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), taskkillTimeout)
		defer cancel()
		kill := exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}

// killLeftovers does nothing on Windows: once cmd exited, taskkill /T can no
// longer find the descendants through it, so a descendant that outlived a
// successful command and held its output pipes is left running.
func killLeftovers(*exec.Cmd) {}
