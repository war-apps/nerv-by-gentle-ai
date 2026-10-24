package deployrun

import "testing"

// The note is JSON.stringify({channels: [channel]}): null for the default
// channel, no HTML escaping.
func TestChannelNote_IsSemanticReleases(t *testing.T) {
	for channel, want := range map[string]string{"": `{"channels":[null]}`, "beta": `{"channels":["beta"]}`, "a<b": `{"channels":["a<b"]}`} {
		if got := channelNote(channel); got != want {
			t.Errorf("channelNote(%q) = %s, want %s", channel, got, want)
		}
	}
}

// The message is compared as git commit -m stores it.
func TestCleanupWhitespace_IsGitsWhitespaceMode(t *testing.T) {
	in := "\n\nchore(release): 1.0.0 [skip ci]  \n\n\n## 1.0.0\t\n\n* a\n\n"
	if got, want := cleanupWhitespace(in), "chore(release): 1.0.0 [skip ci]\n\n## 1.0.0\n\n* a"; got != want {
		t.Errorf("cleanup %q, want %q", got, want)
	}
}
