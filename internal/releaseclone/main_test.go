package releaseclone

import (
	"context"
	"os"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseplan"
)

// TestMain lets the version engine run through this test binary, as nerv
// runs it: the workspace calls {nerv} (os.Executable, the test binary
// here) as `<nerv> provider release-conventional-commits <operation>`.
func TestMain(m *testing.M) {
	if len(os.Args) == 4 && os.Args[1] == "provider" {
		os.Exit(releaseplan.RunNative(context.Background(), os.Args[2], os.Args[3], os.Stdin, os.Stdout, os.Stderr, releaseplan.NativeDeps{}))
	}
	os.Exit(m.Run())
}
