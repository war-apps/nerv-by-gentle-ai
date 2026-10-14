package semrel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteOriginRewrite_RefusesLineBreaksAndNUL(t *testing.T) {
	for name, url := range map[string]string{
		"LF":  "https://bot:tok@host/r.git\n[core]\n\thooksPath = /tmp",
		"CR":  "https://bot:tok@host/r.git\r",
		"NUL": "https://bot:tok@host/r\x00.git",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			for _, args := range [][2]string{{url, "https://host/r.git"}, {"https://bot:tok@host/r.git", url}} {
				if err := writeOriginRewrite(path, args[0], args[1]); err == nil {
					t.Errorf("writeOriginRewrite(%q, %q) = nil, want a refusal", args[0], args[1])
				}
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("config written despite the refusal: %v", err)
			}
		})
	}
}

// TestWriteOriginRewrite_GitReadsBackTheExactURLs proves with git itself
// that backslashes and double quotes are escaped so git parses the rule to
// the original URLs.
func TestWriteOriginRewrite_GitReadsBackTheExactURLs(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	realURL := `https://bot:p"a\ss@host/r.git`
	pushURL := `https://host/r"x\#;.git`
	path := filepath.Join(t.TempDir(), "config")
	if err := writeOriginRewrite(path, realURL, pushURL); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "config", "--file", path, "--null", "--get-regexp", `^url\.`).Output()
	if err != nil {
		t.Fatalf("git config: %v", err)
	}
	key, value, _ := strings.Cut(strings.TrimSuffix(string(out), "\x00"), "\n")
	if want := "url." + realURL + ".insteadof"; key != want || value != pushURL {
		t.Errorf("git read %q = %q, want %q = %q", key, value, want, pushURL)
	}
	if info, err := os.Stat(path); err != nil || (os.PathSeparator == '/' && info.Mode().Perm() != 0o600) {
		t.Errorf("config mode: %v %v", info, err)
	}
}
