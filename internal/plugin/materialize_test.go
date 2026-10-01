package plugin_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

func sampleSrc() fstest.MapFS {
	return fstest.MapFS{
		".claude-plugin/plugin.json": &fstest.MapFile{
			Data: []byte(`{"name":"nerv","version":"1.2.3","description":"Test plugin"}`),
		},
		"agents/aoba.md":    &fstest.MapFile{Data: []byte("# Aoba\n")},
		"hooks/hooks.json":  &fstest.MapFile{Data: []byte(`{"hooks":{}}`)},
		"skills/x/SKILL.md": &fstest.MapFile{Data: []byte("# skill\n")},
	}
}

func readDestFile(t *testing.T, dest string, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return data
}

func TestMaterialize_WritesTreeByteExact(t *testing.T) {
	src := sampleSrc()
	dest := t.TempDir()

	result, err := plugin.Materialize(src, dest)
	if err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}

	for rel, mf := range src {
		got := readDestFile(t, dest, "plugin/"+rel)
		if !bytes.Equal(got, mf.Data) {
			t.Errorf("plugin/%s: got %q, want %q", rel, got, mf.Data)
		}
	}

	if len(result.Written) == 0 {
		t.Error("expected Written to report the newly written files")
	}
	if len(result.Unchanged) != 0 {
		t.Errorf("expected no Unchanged files on first run, got %v", result.Unchanged)
	}
	if len(result.Removed) != 0 {
		t.Errorf("expected no Removed files on first run, got %v", result.Removed)
	}

	// marketplace.json
	marketplaceData := readDestFile(t, dest, ".claude-plugin/marketplace.json")
	var marketplace struct {
		Name  string `json:"name"`
		Owner struct {
			Name string `json:"name"`
		} `json:"owner"`
		Plugins []struct {
			Name        string `json:"name"`
			Source      string `json:"source"`
			Description string `json:"description"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(marketplaceData, &marketplace); err != nil {
		t.Fatalf("parsing marketplace.json: %v", err)
	}
	if marketplace.Name != "nerv" {
		t.Errorf("marketplace.name = %q, want %q", marketplace.Name, "nerv")
	}
	if marketplace.Owner.Name != "Walter Rodriguez" {
		t.Errorf("marketplace.owner.name = %q, want %q", marketplace.Owner.Name, "Walter Rodriguez")
	}
	if len(marketplace.Plugins) != 1 {
		t.Fatalf("expected exactly one plugin entry, got %d", len(marketplace.Plugins))
	}
	p := marketplace.Plugins[0]
	if p.Name != "nerv" || p.Source != "./plugin" || p.Description != "Test plugin" {
		t.Errorf("plugin entry = %+v, want name=nerv source=./plugin description=%q", p, "Test plugin")
	}
	if marketplaceData[len(marketplaceData)-1] != '\n' {
		t.Error("expected marketplace.json to end with a trailing newline")
	}
	if !bytes.Contains(marketplaceData, []byte("  \"name\"")) {
		t.Error("expected marketplace.json to use 2-space indentation")
	}
}

func TestMaterialize_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningfully enforced by the Windows filesystem")
	}

	src := sampleSrc()
	dest := t.TempDir()

	if _, err := plugin.Materialize(src, dest); err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}

	fi, err := os.Stat(filepath.Join(dest, "plugin", "agents", "aoba.md"))
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("file perm = %#o, want %#o", fi.Mode().Perm(), 0o644)
	}

	di, err := os.Stat(filepath.Join(dest, "plugin", "agents"))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if di.Mode().Perm() != 0o755 {
		t.Errorf("dir perm = %#o, want %#o", di.Mode().Perm(), 0o755)
	}
}

func TestMaterialize_IsIdempotent(t *testing.T) {
	src := sampleSrc()
	dest := t.TempDir()

	if _, err := plugin.Materialize(src, dest); err != nil {
		t.Fatalf("first Materialize returned error: %v", err)
	}

	result, err := plugin.Materialize(src, dest)
	if err != nil {
		t.Fatalf("second Materialize returned error: %v", err)
	}

	if len(result.Written) != 0 {
		t.Errorf("expected no Written files on the second run, got %v", result.Written)
	}
	if len(result.Removed) != 0 {
		t.Errorf("expected no Removed files on the second run, got %v", result.Removed)
	}
	// files + marketplace.json
	wantUnchanged := len(src) + 1
	if len(result.Unchanged) != wantUnchanged {
		t.Errorf("expected %d Unchanged files on the second run, got %d (%v)", wantUnchanged, len(result.Unchanged), result.Unchanged)
	}
}

func TestMaterialize_RemovesStaleFiles(t *testing.T) {
	src := sampleSrc()
	dest := t.TempDir()

	if _, err := plugin.Materialize(src, dest); err != nil {
		t.Fatalf("first Materialize returned error: %v", err)
	}

	// Simulate a file from a previous version that no longer exists upstream.
	stalePath := filepath.Join(dest, "plugin", "agents", "old-agent.md")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seeding stale file: %v", err)
	}

	result, err := plugin.Materialize(src, dest)
	if err != nil {
		t.Fatalf("second Materialize returned error: %v", err)
	}

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("expected stale file to be removed, stat err = %v", err)
	}

	found := false
	for _, r := range result.Removed {
		if r == "plugin/agents/old-agent.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Removed to report plugin/agents/old-agent.md, got %v", result.Removed)
	}
}

func TestMaterialize_NeverTouchesOutsideDestPluginAndClaudePlugin(t *testing.T) {
	src := sampleSrc()
	dest := t.TempDir()

	sentinelPath := filepath.Join(dest, "untouched.txt")
	if err := os.WriteFile(sentinelPath, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("seeding sentinel file: %v", err)
	}

	if _, err := plugin.Materialize(src, dest); err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}

	data, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatalf("sentinel file was removed or is unreadable: %v", err)
	}
	if string(data) != "keep me" {
		t.Errorf("sentinel file content changed: %q", data)
	}
}

func TestMaterialize_RejectsDestThatIsNotADirectory(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "not-a-dir")
	if err := os.WriteFile(dest, []byte("i am a file"), 0o644); err != nil {
		t.Fatalf("seeding non-directory dest: %v", err)
	}

	if _, err := plugin.Materialize(sampleSrc(), dest); err == nil {
		t.Fatal("expected an error when dest exists and is not a directory")
	}
}

func TestMaterialize_RejectsPathTraversal(t *testing.T) {
	src := fstest.MapFS{
		".claude-plugin/plugin.json": &fstest.MapFile{
			Data: []byte(`{"name":"nerv","version":"1.2.3","description":"Test plugin"}`),
		},
		"../evil.txt": &fstest.MapFile{Data: []byte("evil")},
	}
	dest := t.TempDir()

	if _, err := plugin.Materialize(src, dest); err == nil {
		t.Fatal("expected an error for a path-traversal entry")
	}
}

func TestMaterialize_RealEmbeddedFSMatchesByteForByte(t *testing.T) {
	src := nerv.PluginFS()
	dest := t.TempDir()

	if _, err := plugin.Materialize(src, dest); err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}

	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		want, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(dest, "plugin", filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("reading materialized %s: %v", path, err)
			return nil
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: materialized bytes differ from embedded source", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking embedded plugin FS: %v", err)
	}
}
