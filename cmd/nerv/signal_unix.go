//go:build unix

package main

import (
	"os"
	"syscall"
)

// cancelSignals cancel a running command's context. SIGHUP is included so
// a closed terminal cancels it too.
var cancelSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
