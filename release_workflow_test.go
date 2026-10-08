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

func workflowFiles(t *testing.T) []string {
	t.Helper()
	var paths []string
	for _, pattern := range []string{".github/workflows/*.yml", ".github/workflows/*.yaml"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, matches...)
	}
	return paths
}

func TestWorkflows_NervCapturesSurviveErrexit(t *testing.T) {
	captures := 0
	for _, path := range workflowFiles(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			line = strings.TrimRight(line, " \r")
			if !nervCapture.MatchString(line) {
				continue
			}
			captures++
			if !errexitSafeTail.MatchString(line) {
				t.Errorf("%s:%d: nerv capture must end with `&& exit_code=0 || exit_code=$?`: %s", path, i+1, strings.TrimSpace(line))
			}
			// A bare `exit_code=$?` is only wrong right after a nerv capture:
			// elsewhere it may follow `set +e` or a non-nerv command on purpose,
			// so only the next non-blank line after a matched capture is checked.
			if j, next, ok := nextNonBlank(lines, i+1); ok && bareExitCode.MatchString(next) {
				t.Errorf("%s:%d: bare `exit_code=$?` after a nerv capture never runs under bash -e; use `cmd && exit_code=0 || exit_code=$?`", path, j+1)
			}
		}
	}
	// release.yml captures `release preview` twice and `release guard` once;
	// the lower bound pins those three known captures, so a stale pattern or
	// a dropped capture both fail here.
	if captures < 3 {
		t.Errorf("found %d nerv captures in .github/workflows, want at least 3", captures)
	}
}

// nextNonBlank returns the index and right-trimmed text of the first
// non-blank line at or after start.
func nextNonBlank(lines []string, start int) (int, string, bool) {
	for j := start; j < len(lines); j++ {
		line := strings.TrimRight(lines[j], " \r")
		if strings.TrimSpace(line) != "" {
			return j, line, true
		}
	}
	return 0, "", false
}
