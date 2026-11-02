package upgrade

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckPlatform checks every goos/goarch combination goreleaser builds
// (linux, darwin and windows, each amd64/arm64) is accepted, and a platform
// or architecture goreleaser does not build is refused.
func TestCheckPlatform(t *testing.T) {
	tests := []struct {
		goos, goarch string
		wantErr      bool
	}{
		{"linux", "amd64", false},
		{"linux", "arm64", false},
		{"darwin", "amd64", false},
		{"darwin", "arm64", false},
		{"windows", "amd64", false},
		{"windows", "arm64", false},
		{"freebsd", "amd64", true},
		{"windows", "386", true},
		{"linux", "386", true},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			err := CheckPlatform(tt.goos, tt.goarch)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckPlatform(%q, %q) = %v, want error: %v", tt.goos, tt.goarch, err, tt.wantErr)
			}
		})
	}
}

// TestArchiveName checks the archive extension: .tar.gz everywhere except
// windows, which goreleaser packages as .zip.
func TestArchiveName(t *testing.T) {
	tests := []struct {
		tag, goos, goarch, want string
	}{
		{"v1.2.3", "linux", "amd64", "nerv_1.2.3_linux_amd64.tar.gz"},
		{"v1.2.3", "darwin", "arm64", "nerv_1.2.3_darwin_arm64.tar.gz"},
		{"v1.2.3", "windows", "amd64", "nerv_1.2.3_windows_amd64.zip"},
		{"v1.2.3", "windows", "arm64", "nerv_1.2.3_windows_arm64.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			if got := ArchiveName(tt.tag, tt.goos, tt.goarch); got != tt.want {
				t.Errorf("ArchiveName(%q, %q, %q) = %q, want %q", tt.tag, tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

// tarGzArchive builds a tar.gz holding one file named "nerv" with body.
func tarGzArchive(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "nerv", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// zipArchive builds a .zip holding one file named "nerv.exe" with body.
func zipArchive(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("nerv.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestReplace_NonWindowsRenamesOver keeps today's behavior: the new binary
// is renamed directly over exe, with no ".old" sibling left behind.
func TestReplace_NonWindowsRenamesOver(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			dir := t.TempDir()
			exe := filepath.Join(dir, "nerv")
			if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			archive := tarGzArchive(t, "new")

			if err := Replace(archive, exe, goos); err != nil {
				t.Fatalf("Replace: %v", err)
			}
			got, err := os.ReadFile(exe)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "new" {
				t.Errorf("exe = %q, want %q", got, "new")
			}
			if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
				t.Errorf("exe.old exists on %s, want none", goos)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Errorf("dir holds %d entries, want only exe", len(entries))
			}
		})
	}
}

// TestReplace_WindowsExtractsZipAndPlacesAsideOld proves the windows path
// reads nerv.exe out of the zip and installs it by renaming the running
// exe aside to exe.old first, instead of renaming the new binary directly
// over it (which Windows refuses while it is running).
func TestReplace_WindowsExtractsZipAndPlacesAsideOld(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "nerv.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := zipArchive(t, "new")

	if err := Replace(archive, exe, "windows"); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("exe = %q, want %q", got, "new")
	}
	old, err := os.ReadFile(exe + ".old")
	if err != nil {
		t.Fatalf("exe.old: %v", err)
	}
	if string(old) != "old" {
		t.Errorf("exe.old = %q, want the original %q", old, "old")
	}
}

// TestReplace_ZipMissingNervExe reports a clear error when the archive has
// no nerv.exe entry.
func TestReplace_ZipMissingNervExe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	if _, err := zw.Create("README.md"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	exe := filepath.Join(t.TempDir(), "nerv.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	err = Replace(path, exe, "windows")
	if err == nil {
		t.Fatal("want an error when nerv.exe is missing from the archive")
	}
	got, readErr := os.ReadFile(exe)
	if readErr != nil || string(got) != "old" {
		t.Errorf("exe = %q, %v, want it untouched", got, readErr)
	}
}

// TestPlaceWindows_RestoresOriginalWhenPlacementFails checks that when the
// new binary cannot be placed at exe, the original (renamed aside) is
// restored so exe is never left missing.
func TestPlaceWindows_RestoresOriginalWhenPlacementFails(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "nerv.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	missingTmp := filepath.Join(dir, "does-not-exist")

	if err := placeWindows(missingTmp, exe); err == nil {
		t.Fatal("want an error when placing the new binary fails")
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("exe missing after a failed placement: %v", err)
	}
	if string(got) != "old" {
		t.Errorf("exe = %q, want the original restored", got)
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Errorf("exe.old left behind after a restore: %v", err)
	}
}

// TestPlaceWindows_RemovesStaleOldFirst checks a leftover exe.old from an
// earlier upgrade does not block renaming the current exe aside.
func TestPlaceWindows_RemovesStaleOldFirst(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "nerv.exe")
	tmp := filepath.Join(dir, "new")
	if err := os.WriteFile(exe, []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe+".old", []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := placeWindows(tmp, exe); err != nil {
		t.Fatalf("placeWindows: %v", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil || string(got) != "new" {
		t.Errorf("exe = %q, %v, want %q", got, err, "new")
	}
	old, err := os.ReadFile(exe + ".old")
	if err != nil || string(old) != "current" {
		t.Errorf("exe.old = %q, %v, want the just-replaced %q, not the stale one", old, err, "current")
	}
}

// TestCleanupStaleReplacement_RemovesLeftoverOld checks the best-effort
// cleanup a later run performs: a leftover exe.old from an earlier windows
// replacement is removed.
func TestCleanupStaleReplacement_RemovesLeftoverOld(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "nerv.exe")
	old := exe + ".old"
	if err := os.WriteFile(old, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	CleanupStaleReplacement(exe)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("exe.old still present after cleanup: %v", err)
	}
}

// TestCleanupStaleReplacement_NoOldIsHarmless checks the cleanup is safe
// to call unconditionally (e.g. on Linux/macOS, where Replace never
// creates a ".old") even when there is nothing to remove.
func TestCleanupStaleReplacement_NoOldIsHarmless(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "nerv")
	CleanupStaleReplacement(exe) // must not panic
}
