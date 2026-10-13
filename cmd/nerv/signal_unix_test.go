//go:build unix

package main

import (
	"context"
	"syscall"
	"testing"
	"time"
)

// TestSignalContext_CancelsOnInterruptTerminateAndHangup sends each signal to
// this process and expects the real signal context to be cancelled (and the
// process to survive: the signal is caught, not fatal). The semantic-release
// engine starts its commands in their own session, so these signals reach
// them only through this context.
func TestSignalContext_CancelsOnInterruptTerminateAndHangup(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			ctx, stop := signalContext(context.Background())
			defer stop()

			if err := syscall.Kill(syscall.Getpid(), sig); err != nil {
				t.Fatal(err)
			}
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
				t.Fatalf("context not cancelled 5s after %s", sig)
			}
		})
	}
}

func TestDefaultOptions_WireTheRealSignalContextAndEngine(t *testing.T) {
	opts := defaultOptions()
	if opts.SignalContext == nil || opts.ComputeVersions == nil {
		t.Fatal("defaultOptions leaves the signal context or the engine unset")
	}
}
