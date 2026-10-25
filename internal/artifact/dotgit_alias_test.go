package artifact

import "testing"

// Git refuses these spellings of .git (core.protectNTFS, core.protectHFS);
// a release file must not reach the clone's git directory through them.
func TestSafeReleasePath_RefusesGitDirAliases(t *testing.T) {
	for _, p := range []string{
		".git/config", ".GIT/config", "sub/.git/hooks/post-commit",
		".git./config", ".git /config", ".git. . /config", // NTFS trailing dots and spaces
		"GIT~1/config", "git~2/hooks/x", // NTFS 8.3 short names
		".git::$INDEX_ALLOCATION/config", ".git:stream", // NTFS alternate data streams
		".g\u200cit/config", "\ufeff.git/config", ".git\u200d/config", // HFS+ ignorable code points
	} {
		if SafeReleasePath(p) {
			t.Errorf("SafeReleasePath(%q) = true, want false", p)
		}
	}
	for _, p := range []string{"CHANGELOG.md", "src/App.csproj", ".github/workflows/ci.yml", "gitx/a", ".gitignore", "a/.gitkeep"} {
		if !SafeReleasePath(p) {
			t.Errorf("SafeReleasePath(%q) = false, want true", p)
		}
	}
}
