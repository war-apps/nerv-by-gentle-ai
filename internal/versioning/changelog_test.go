package versioning

import "testing"

func TestPrependChangelog(t *testing.T) {
	section := "## 1.1.0 (2026-10-23)\n\n### Features\n\n* b (bbbbbbb)\n"
	cases := []struct{ name, current, title, want string }{
		{"new file", "", "", section},
		{"existing file", "## 1.0.0 (2026-10-01)\n\n* a\n\n", "", section + "\n## 1.0.0 (2026-10-01)\n\n* a\n"},
		{"title kept on top", "# Changelog\n\n## 1.0.0\n", "# Changelog", "# Changelog\n\n" + section + "\n## 1.0.0\n"},
		{"title added to a new file", "  \n", "# Changelog", "# Changelog\n\n" + section},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PrependChangelog(c.current, section, c.title); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}
