//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// stdoutColor reports whether stdout is a console that shows ANSI colors,
// turning on its virtual terminal processing when it is off (consoles
// older than Windows 10 refuse it: no colors there).
func stdoutColor() bool {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
