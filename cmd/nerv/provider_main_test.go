package main

import (
	"os"
	"testing"
)

// TestMain lets the release flow under test run its native providers
// through this test binary, as the real nerv runs them: nerv release and
// nerv version next call {nerv} (os.Executable, the test binary here) as
// `<nerv> provider release-<name> <operation>`, which runs the hidden
// command instead of the tests. The test flags always come first
// otherwise, so a normal test run never matches.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "provider" {
		opts := defaultOptions()
		os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, opts))
	}
	os.Exit(m.Run())
}
