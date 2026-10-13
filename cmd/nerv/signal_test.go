package main

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// fakeSignals replaces signal.Notify / signal.Stop for one test (tests that
// use it must not run in parallel). It returns the channel signalContext
// registered and a counter of Stop calls on it.
func fakeSignals(t *testing.T) (registered func() chan<- os.Signal, stops *atomic.Int32) {
	t.Helper()
	oldNotify, oldStop := signalNotify, signalStop
	t.Cleanup(func() { signalNotify, signalStop = oldNotify, oldStop })

	chans := make(chan chan<- os.Signal, 1)
	stops = &atomic.Int32{}
	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) { chans <- c }
	signalStop = func(chan<- os.Signal) { stops.Add(1) }
	return func() chan<- os.Signal {
		select {
		case c := <-chans:
			return c
		case <-time.After(5 * time.Second):
			t.Fatal("signalContext registered no channel")
			return nil
		}
	}, stops
}

// TestSignalContext_FirstSignalReleasesTheHandler: once the first signal has
// cancelled the run, signals are no longer caught, so a second Ctrl-C gets
// the default behaviour (the process exits at once) instead of being
// swallowed until the command returns.
func TestSignalContext_FirstSignalReleasesTheHandler(t *testing.T) {
	registered, stops := fakeSignals(t)

	ctx, stop, received := signalContext(context.Background())
	defer stop()
	registered() <- syscall.SIGINT

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("context not cancelled after the signal")
	}
	if n := stops.Load(); n != 1 {
		t.Errorf("signal handler released %d times when the context was cancelled, want 1 (before the cancellation)", n)
	}
	if got := received(); got != syscall.SIGINT {
		t.Errorf("received() = %v, want SIGINT", got)
	}
	stop()
	if n := stops.Load(); n != 1 {
		t.Errorf("signal handler released %d times after stop, want still 1", n)
	}
}

func TestSignalContext_ParentCancellationReleasesTheHandler(t *testing.T) {
	registered, stops := fakeSignals(t)
	parent, cancel := context.WithCancel(context.Background())

	ctx, stop, received := signalContext(parent)
	defer stop()
	registered()
	cancel()

	<-ctx.Done()
	deadline := time.Now().Add(5 * time.Second)
	for stops.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := stops.Load(); n != 1 {
		t.Errorf("signal handler released %d times, want 1", n)
	}
	if got := received(); got != nil {
		t.Errorf("received() = %v, want nil (no signal)", got)
	}
}

func TestSignalContext_StopWithoutASignalReleasesTheHandler(t *testing.T) {
	registered, stops := fakeSignals(t)

	ctx, stop, _ := signalContext(context.Background())
	registered()
	stop()

	if ctx.Err() == nil {
		t.Error("stop did not cancel the context")
	}
	deadline := time.Now().Add(5 * time.Second)
	for stops.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if n := stops.Load(); n != 1 {
		t.Errorf("signal handler released %d times, want 1", n)
	}
}
