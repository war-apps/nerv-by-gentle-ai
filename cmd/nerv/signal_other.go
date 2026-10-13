//go:build !unix

package main

import (
	"os"
	"syscall"
)

// cancelSignals cancel a running command's context. There is no SIGHUP
// outside Unix; on Windows the engine starts its commands in a new process
// group, which console Ctrl-C does not reach, so os.Interrupt here is how
// Ctrl-C stops them.
var cancelSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
