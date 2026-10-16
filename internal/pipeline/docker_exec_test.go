package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// TestDocker_AFailedBuildReportsItsCauseWithTheRealRunner runs a fake
// docker script through the real process runner: BuildKit writes its
// errors on stderr, so the step's error must carry both streams' last
// lines while the output is streamed.
func TestDocker_AFailedBuildReportsItsCauseWithTheRealRunner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake docker is a shell script")
	}
	bin := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = build ]; then
  echo "#13 4.571 restoring (stdout)"
  echo "#13 5.774 error NU1903: Package 'Pkcs' 9.0.4 is vulnerable (stderr)" >&2
  echo "ERROR: failed to solve: dotnet restore did not complete successfully (stderr)" >&2
  exit 1
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out strings.Builder
	d := dockerStep(&fakeDocker{}, &fakeArgs{values: []string{"130.0"}}, release("main", "2.9.0", "", "v2.9.0"))
	d.Proc, d.Dir, d.Output = proc.ExecRunner{}, t.TempDir(), &out
	r := d.Run(context.Background())
	for _, want := range []string{"ghcr.io/acme/api", "restoring (stdout)", "error NU1903", "failed to solve"} {
		if !strings.Contains(r.Error, want) {
			t.Errorf("error %q lacks %q", r.Error, want)
		}
	}
	if !strings.Contains(out.String(), "failed to solve") {
		t.Errorf("the build's stderr was not streamed: %q", out.String())
	}
}
