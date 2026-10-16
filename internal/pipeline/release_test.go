package pipeline

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseOutputs_WrittenSafelyAndRead(t *testing.T) {
	repo := t.TempDir()
	o := ReleaseOutputs{Step: "release", Status: StatusPassed, Published: true, Version: "1.1.0-alpha.1", Channel: "alpha", GitTag: "v1.1.0-alpha.1",
		Outputs: map[string]string{"new_release_published": "true"}}
	path, err := WriteReleaseOutputs(repo, o)
	if err != nil {
		t.Fatal(err)
	}
	if path != ReleaseOutputsPath(repo) || path != filepath.Join(repo, ".nerv", "artifacts", "pipeline", "release.json") {
		t.Errorf("path %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back ReleaseOutputs
	if err := json.Unmarshal(data, &back); err != nil || back.Version != "1.1.0-alpha.1" || !back.Published || back.Outputs["new_release_published"] != "true" {
		t.Errorf("read back %+v %v", back, err)
	}
	if ignore, _ := os.ReadFile(filepath.Join(repo, ".nerv", "artifacts", ".gitignore")); strings.TrimSpace(string(ignore)) != "*" {
		t.Errorf(".gitignore %q", ignore)
	}
	if err := RemoveReleaseOutputs(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("not removed: %v", err)
	}
	if err := RemoveReleaseOutputs(repo); err != nil {
		t.Errorf("removing twice: %v", err)
	}
}

func TestReleaseOutputs_RefusedBehindASymlinkOutside(t *testing.T) {
	for _, link := range []string{".nerv", filepath.Join(".nerv", "artifacts"), filepath.Join(".nerv", "artifacts", "pipeline")} {
		t.Run(link, func(t *testing.T) {
			repo, outside := t.TempDir(), t.TempDir()
			if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, link)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(repo, link)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := WriteReleaseOutputs(repo, ReleaseOutputs{Step: "release"}); err == nil || !strings.Contains(err.Error(), "outside") {
				t.Errorf("write: err = %v", err)
			}
			if err := RemoveReleaseOutputs(repo); err == nil || !strings.Contains(err.Error(), "outside") {
				t.Errorf("remove: err = %v", err)
			}
			if entries, _ := os.ReadDir(outside); len(entries) != 0 {
				t.Errorf("created outside: %v", entries)
			}
		})
	}
}

func TestPrefixWriter(t *testing.T) {
	var b strings.Builder
	w := NewPrefixWriter(&b, "[pipeline:release] ")
	w.Write([]byte("one\ntw"))
	w.Write([]byte("o\nthree"))
	if got := b.String(); got != "[pipeline:release] one\n[pipeline:release] two\n" {
		t.Errorf("before flush %q", got)
	}
	w.Flush()
	if got := b.String(); !strings.HasSuffix(got, "[pipeline:release] three\n") {
		t.Errorf("after flush %q", got)
	}
}

// failOnce fails its first write.
type failOnce struct {
	strings.Builder
	failed bool
}

func (f *failOnce) Write(b []byte) (int, error) {
	if !f.failed {
		f.failed = true
		return 0, errors.New("disk full")
	}
	return f.Builder.Write(b)
}

func TestPrefixWriter_DropsTheLineThatFailed(t *testing.T) {
	out := &failOnce{}
	w := NewPrefixWriter(out, "> ")
	if _, err := w.Write([]byte("lost\n")); err == nil {
		t.Fatal("the write error was not returned")
	}
	w.Write([]byte("kept\n"))
	w.Write([]byte("tail"))
	w.Flush()
	if got := out.String(); got != "> kept\n> tail\n" {
		t.Errorf("output %q: the failed line must not be written again", got)
	}
}
