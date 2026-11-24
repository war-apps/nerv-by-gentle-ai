package main

import (
	"bytes"
	"strings"
	"testing"
)

// The release commands moved to the nerv-release tool (nerv release is now
// nerv-release build, nerv release setup is nerv-release setup): each answers with
// one stderr line naming the nerv-release command that replaces it and how
// to install the tool, exits 2 and writes nothing to stdout.
func TestRun_MovedReleaseCommandsPointToNervRelease(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"release"}, "nerv-release build"},
		{[]string{"release", "--dry-run"}, "nerv-release build"},
		{[]string{"release", "setup", "--import"}, "nerv-release setup --import"},
		{[]string{"deploy", "--artifact", "x"}, "nerv-release deploy"},
		{[]string{"version", "next"}, "nerv-release version next"},
		{[]string{"version", "next", "--format", "json"}, "nerv-release version next"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run(tc.args, &stdout, &stderr, testOptions(t.TempDir()))

			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			msg := stderr.String()
			if strings.Count(msg, "\n") != 1 || !strings.HasSuffix(msg, "\n") {
				t.Errorf("stderr = %q, want exactly one line", msg)
			}
			for _, part := range []string{"moved to the nerv-release tool", "nerv install --tools nerv-release", tc.want} {
				if !strings.Contains(msg, part) {
					t.Errorf("stderr = %q, want it to contain %q", msg, part)
				}
			}
			if strings.Contains(msg, "Usage:") {
				t.Errorf("stderr = %q, want the moved notice, not the usage", msg)
			}
		})
	}
}

// The hidden nerv provider command is gone: it is an unknown command.
func TestRun_ProviderIsAnUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"provider", "release-bash", "check"}, &stdout, &stderr, testOptions(t.TempDir()))

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != usage {
		t.Errorf("stderr = %q, want the usage", stderr.String())
	}
}

// The usage no longer lists the moved commands; it keeps nerv version and
// nerv maintainer-release.
func TestUsage_ListsNoMovedReleaseCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr, testOptions(t.TempDir())); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	out := stdout.String()
	for _, gone := range []string{"\n  release", "\n  deploy", "version next", "release setup", "provider"} {
		if strings.Contains(out, gone) {
			t.Errorf("usage still lists %q:\n%s", gone, out)
		}
	}
	for _, kept := range []string{"\n  version [--json]", "\n  maintainer-release "} {
		if !strings.Contains(out, kept) {
			t.Errorf("usage lacks %q:\n%s", kept, out)
		}
	}
}
