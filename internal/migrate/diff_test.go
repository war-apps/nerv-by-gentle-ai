package migrate

import "testing"

func TestUnifiedDiff_HunksWithContext(t *testing.T) {
	// Changes more than two contexts apart make two hunks.
	old := []byte("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nl\n")
	new := []byte("a\nb\nc\nD\ne\nf\ng\nh\ni\nj\nl\nk\n")
	want := "--- a/x.yml\n+++ b/x.yml\n" +
		"@@ -1,7 +1,7 @@\n a\n b\n c\n-d\n+D\n e\n f\n g\n" +
		"@@ -9,3 +9,4 @@\n i\n j\n l\n+k\n"
	if got := UnifiedDiff("x.yml", old, new); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := UnifiedDiff("x.yml", old, old); got != "" {
		t.Errorf("equal files: %q", got)
	}
	if got := UnifiedDiff("n.yml", nil, []byte("x\n")); got != "--- /dev/null\n+++ b/n.yml\n@@ -0,0 +1 @@\n+x\n" {
		t.Errorf("new file: %q", got)
	}
}
