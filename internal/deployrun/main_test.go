package deployrun

import (
	"context"
	"os"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/deploy"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// TestMain lets the fake nerv of the tests (helpers_test.go) run the real
// `nerv provider deploy-ssh-compose preflight` and `nerv provider
// deploy-bash <op>` through this test binary: with NERV_FAKE_PROVIDER=1 the
// binary answers the provider and operation named by its last two
// arguments and exits instead of running the tests.
func TestMain(m *testing.M) {
	if os.Getenv("NERV_FAKE_PROVIDER") == "1" {
		provider, op := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
		if provider == "deploy-bash" {
			os.Exit(deploy.RunBashOp(context.Background(), op, os.Stdin, os.Stdout, deploy.BashDeps{Output: os.Stderr}))
		}
		os.Exit(deploy.RunComposeOp(context.Background(), op, os.Stdin, os.Stdout, deploy.ComposeDeps{Proc: proc.ExecRunner{}, Output: os.Stderr}))
	}
	os.Exit(m.Run())
}
