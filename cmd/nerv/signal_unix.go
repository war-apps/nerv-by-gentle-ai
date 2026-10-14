//go:build unix

package main

import (
	"os"
	"syscall"
)

// cancelSignals cancel a running command's context. SIGHUP is included
// because the semantic-release engine starts git, npm and node in their own
// session, so a closed terminal no longer reaches them directly.
var cancelSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
