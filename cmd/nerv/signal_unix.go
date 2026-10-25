//go:build unix

package main

import (
	"os"
	"syscall"
)

// cancelSignals cancel a running command's context. SIGHUP is included
// because nerv release and nerv deploy start git and the build and provider
// commands in their own session (internal/proc), so a closed terminal no
// longer reaches them directly.
var cancelSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
