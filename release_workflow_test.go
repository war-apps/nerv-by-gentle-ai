package nerv_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// GitHub runs workflow bash steps with -e, so a failing command substitution
// aborts the step before a following `exit_code=$?` can read it. Every nerv
// CLI capture must record its exit code in the same command list instead.
var (
	nervCapture     = regexp.MustCompile(`^\s*output=\$\(go run \./cmd/nerv .*\)`)
	errexitSafeTail = regexp.MustCompile(`\) && exit_code=0 \|\| exit_code=\$\?$`)
	bareExitCode    = regexp.MustCompile(`^\s*exit_code=\$\?\s*$`)
)

func TestWorkflows_NervCapturesSurviveErrexit(t *testing.T) {
	paths, err := filepath.Glob(".github/workflows/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	captures := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			line = strings.TrimRight(line, " \r")
			if bareExitCode.MatchString(line) {
				t.Errorf("%s:%d: bare `exit_code=$?` never runs under bash -e; use `cmd && exit_code=0 || exit_code=$?`", path, i+1)
			}
			if !nervCapture.MatchString(line) {
				continue
			}
			captures++
			if !errexitSafeTail.MatchString(line) {
				t.Errorf("%s:%d: nerv capture must end with `&& exit_code=0 || exit_code=$?`: %s", path, i+1, strings.TrimSpace(line))
			}
		}
	}
	// release.yml captures `release preview` twice and `release guard` once;
	// zero matches would mean the patterns above went stale.
	if captures < 3 {
		t.Errorf("found %d nerv captures in .github/workflows, want at least 3", captures)
	}
}
