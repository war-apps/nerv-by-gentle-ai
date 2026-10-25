package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// sample is a full input: one image tar, one file and one directory output,
// two release files.
func sample(t *testing.T) Input {
	t.Helper()
	src := t.TempDir()
	write := func(rel, content string) string {
		p := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("site/index.html", "<html>")
	write("site/css/app.css", "body{}")
	return Input{
		Manifest: Manifest{
			Project: "api", Version: "1.4.0-alpha.2", Channel: "alpha", Tag: "api-v1.4.0-alpha.2",
			BaseCommit: strings.Repeat("a", 40), Branch: "develop", BuildProvider: "docker-compose",
			CreatedAt: time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC),
			Images:    []string{"ghcr.io/acme/api:1.4.0-alpha.2"},
		},
		Notes:            "## 1.4.0-alpha.2\n\n* a change\n",
		ChangelogSection: "## 1.4.0-alpha.2 (2026-10-23)\n\n* a change\n",
		Outputs: []OutputSource{
			{Path: "api.tar", Kind: KindImageTar, From: write("api.tar", "image bytes")},
			{Path: "Api.1.4.0.nupkg", Kind: KindFile, From: write("Api.nupkg", "zip bytes")},
			{Path: "site", Kind: KindDir, From: filepath.Join(src, "site")},
		},
		ReleaseFiles: []ReleaseFileSource{
			{Path: "api/CHANGELOG.md", Content: []byte("# Changelog\n\n## 1.4.0-alpha.2\n")},
			{Path: "api/src/Api.csproj", Content: []byte("<Project><Version>1.4.0-alpha.2</Version></Project>\n")},
		},
	}
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestWrite_LaysOutTheArtifactUnderTheRepository(t *testing.T) {
	repo := t.TempDir()
	in := sample(t)
	dir, err := Write(repo, in, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(repo, ".nerv", "artifacts", "api", "1.4.0-alpha.2"); dir != want {
		t.Errorf("dir %s, want %s", dir, want)
	}
	for rel, want := range map[string]string{
		"notes.md":                         in.Notes,
		"CHANGELOG.section.md":             in.ChangelogSection,
		"outputs/api.tar":                  "image bytes",
		"outputs/Api.1.4.0.nupkg":          "zip bytes",
		"outputs/site/index.html":          "<html>",
		"outputs/site/css/app.css":         "body{}",
		"release-files/api/CHANGELOG.md":   "# Changelog\n\n## 1.4.0-alpha.2\n",
		"release-files/api/src/Api.csproj": "<Project><Version>1.4.0-alpha.2</Version></Project>\n",
		"../../.gitignore":                 "*\n",
	} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil || string(data) != want {
			t.Errorf("%s: %q %v, want %q", rel, data, err, want)
		}
	}

	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"schema": "nerv.release-artifact/v1", "project": "api", "version": "1.4.0-alpha.2", "channel": "alpha",
		"tag": "api-v1.4.0-alpha.2", "base_commit": strings.Repeat("a", 40), "branch": "develop",
		"build_provider": "docker-compose", "created_at": "2026-10-23T12:00:00Z",
	} {
		if doc[key] != want {
			t.Errorf("manifest %s = %v, want %v", key, doc[key], want)
		}
	}

	m, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantOutputs := []Output{
		{Path: "outputs/api.tar", Kind: KindImageTar, SHA256: sum("image bytes")},
		{Path: "outputs/Api.1.4.0.nupkg", Kind: KindFile, SHA256: sum("zip bytes")},
		{Path: "outputs/site", Kind: KindDir, SHA256: sum(sum("body{}") + "  css/app.css\n" + sum("<html>") + "  index.html\n")},
	}
	if !slices.Equal(m.Outputs, wantOutputs) {
		t.Errorf("outputs\n %+v\nwant\n %+v", m.Outputs, wantOutputs)
	}
	if !slices.Equal(m.Images, []string{"ghcr.io/acme/api:1.4.0-alpha.2"}) {
		t.Errorf("images %v", m.Images)
	}
	wantFiles := []ReleaseFile{
		{Path: "api/CHANGELOG.md", Stored: "release-files/api/CHANGELOG.md", SHA256: sum("# Changelog\n\n## 1.4.0-alpha.2\n")},
		{Path: "api/src/Api.csproj", Stored: "release-files/api/src/Api.csproj", SHA256: sum("<Project><Version>1.4.0-alpha.2</Version></Project>\n")},
	}
	if !slices.Equal(m.ReleaseFiles, wantFiles) {
		t.Errorf("release files\n %+v\nwant\n %+v", m.ReleaseFiles, wantFiles)
	}
	if _, err := Verify(dir); err != nil {
		t.Errorf("verify: %v", err)
	}
}

func TestWrite_KeepsAnExistingGitignore(t *testing.T) {
	repo := t.TempDir()
	ignore := filepath.Join(repo, ".nerv", "artifacts", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(ignore), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignore, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(repo, sample(t), false); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(ignore); string(data) != "mine\n" {
		t.Errorf(".gitignore %q, want it untouched", data)
	}
}

func TestWrite_RefusesAnExistingVersionUnlessForced(t *testing.T) {
	repo := t.TempDir()
	in := sample(t)
	dir, err := Write(repo, in, false)
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "outputs", "stale.txt")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(repo, in, false); !errors.Is(err, ErrExists) || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("second write: %v, want ErrExists naming --force", err)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("a refused write touched the artifact: %v", err)
	}
	in.Notes = "new notes\n"
	if _, err := Write(repo, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("forced write kept a stale output: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "notes.md")); string(data) != "new notes\n" {
		t.Errorf("notes %q after --force", data)
	}
	entries, _ := os.ReadDir(filepath.Dir(dir))
	if len(entries) != 1 {
		t.Errorf("project directory holds %d entries, want only the version (no temporary leftovers)", len(entries))
	}
}

func TestVerify_DetectsAChangedOutputOrReleaseFile(t *testing.T) {
	for _, rel := range []string{"outputs/api.tar", "outputs/site/css/app.css", "release-files/api/CHANGELOG.md"} {
		t.Run(rel, func(t *testing.T) {
			dir, err := Write(t.TempDir(), sample(t), false)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte("tampered"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(dir); err == nil || !strings.Contains(err.Error(), "sha256") {
				t.Errorf("verify: %v, want a sha256 mismatch", err)
			}
		})
	}
	dir, err := Write(t.TempDir(), sample(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "outputs", "api.tar")); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(dir); err == nil || !strings.Contains(err.Error(), "outputs/api.tar") {
		t.Errorf("verify of a missing output: %v", err)
	}
}

func TestRead_RefusesAnotherSchemaOrAPathLeavingTheArtifact(t *testing.T) {
	tests := []struct {
		name string
		edit func(m map[string]any)
		want string
	}{
		{"schema", func(m map[string]any) { m["schema"] = "nerv.release-artifact/v2" }, "schema"},
		// S1: an artifact nerv ship wrote (5.0.0-alpha.1) says how to get a
		// current one.
		{"alpha.1 schema", func(m map[string]any) { m["schema"] = "nerv.ship-artifact/v1" },
			`schema "nerv.ship-artifact/v1": this artifact was made by nerv 5.0.0-alpha.1 (nerv ship); rebuild it with nerv release`},
		{"output path", func(m map[string]any) {
			m["outputs"] = []any{map[string]any{"path": "../escape", "kind": "file", "sha256": sum("")}}
		}, "../escape"},
		{"release file", func(m map[string]any) {
			m["release_files"] = []any{map[string]any{"path": "/etc/passwd", "stored": "release-files/x", "sha256": sum("")}}
		}, "/etc/passwd"},
		{"kind", func(m map[string]any) {
			m["outputs"] = []any{map[string]any{"path": "outputs/x", "kind": "zip", "sha256": sum("")}}
		}, "kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := Write(t.TempDir(), sample(t), false)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "manifest.json")
			raw, _ := os.ReadFile(path)
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			tt.edit(m)
			raw, _ = json.Marshal(m)
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Read(dir); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("read: %v, want an error naming %q", err, tt.want)
			}
		})
	}
}

func TestWrite_RefusesUnsafeInput(t *testing.T) {
	tests := []struct {
		name string
		edit func(in *Input)
		want string
	}{
		{"version with a slash", func(in *Input) { in.Manifest.Version = "1.0/../../x" }, "version"},
		{"project name", func(in *Input) { in.Manifest.Project = "../api" }, "project"},
		{"output path escaping", func(in *Input) { in.Outputs[0].Path = "../x.tar" }, "../x.tar"},
		{"release file escaping", func(in *Input) { in.ReleaseFiles[0].Path = "../CHANGELOG.md" }, "../CHANGELOG.md"},
		{"duplicate output", func(in *Input) { in.Outputs[1].Path = "api.tar" }, "twice"},
		{"missing source", func(in *Input) { in.Outputs[0].From = filepath.Join(t.TempDir(), "nope") }, "api.tar"},
		{"kind", func(in *Input) { in.Outputs[0].Kind = "zip" }, "kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			in := sample(t)
			tt.edit(&in)
			if _, err := Write(repo, in, false); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("write: %v, want an error naming %q", err, tt.want)
			}
			if entries, _ := os.ReadDir(filepath.Join(repo, ".nerv", "artifacts", "api")); len(entries) != 0 {
				t.Errorf("a refused write left %d entries", len(entries))
			}
		})
	}
}

func TestWrite_RefusesArtifactsLinkedOutsideTheRepository(t *testing.T) {
	repo, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".nerv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".nerv", "artifacts")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := Write(repo, sample(t), false); err == nil || !strings.Contains(err.Error(), "outside the repository") {
		t.Errorf("write: %v", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("wrote %d entries outside the repository", len(entries))
	}
}

func TestDir_IsTheVersionDirectory(t *testing.T) {
	if got, want := Dir("/repo", "web", "2.0.0"), filepath.Join("/repo", ".nerv", "artifacts", "web", "2.0.0"); got != want {
		t.Errorf("Dir %s, want %s", got, want)
	}
}
