package deployrun

import (
	"context"
	"os"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/deploy"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// TestMain lets the fake nerv of the tests (helpers_test.go) run the real
// `nerv provider deploy-ssh-compose preflight` through this test binary:
// with NERV_FAKE_PROVIDER=1 the binary answers the operation named by its
// last argument and exits instead of running the tests.
func TestMain(m *testing.M) {
	if os.Getenv("NERV_FAKE_PROVIDER") == "1" {
		op := os.Args[len(os.Args)-1]
		os.Exit(deploy.RunComposeOp(context.Background(), op, os.Stdin, os.Stdout, deploy.ComposeDeps{Proc: proc.ExecRunner{}, Output: os.Stderr}))
	}
	os.Exit(m.Run())
}
