package shipplan

import (
	"strings"
	"testing"
)

func TestCompareVersions_FollowsSemverPrecedence(t *testing.T) {
	ordered := []string{
		"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2",
		"1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0",
	}
	for i := range ordered {
		for j := range ordered {
			got, err := CompareVersions(ordered[i], ordered[j])
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			switch {
			case i < j:
				want = -1
			case i > j:
				want = 1
			}
			if got != want {
				t.Errorf("CompareVersions(%s, %s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
	if c, _ := CompareVersions("1.0.0+build.1", "1.0.0"); c != 0 {
		t.Errorf("build metadata counted: %d", c)
	}
	for _, bad := range []string{"1.0", "v1.0.0", "1.0.0-", "01.0.0", "1.0.0-01", "1.0.0 ", ""} {
		if _, err := CompareVersions(bad, "1.0.0"); err == nil || !strings.Contains(err.Error(), "semantic version") {
			t.Errorf("CompareVersions(%q) = %v, want an error", bad, err)
		}
	}
}

func TestVersionFromTag(t *testing.T) {
	tests := []struct {
		format, tag, want string
		ok                bool
	}{
		{"v${version}", "v1.2.3", "1.2.3", true},
		{"api-v${version}", "api-v1.2.3-rc.1", "1.2.3-rc.1", true},
		{"api-v${version}", "web-v1.2.3", "", false},
		{"v${version}", "v1.2", "", false},
		{"lib@${version}", "lib@2.0.0", "2.0.0", true},
	}
	for _, tt := range tests {
		got, ok := VersionFromTag(tt.format, tt.tag)
		if got != tt.want || ok != tt.ok {
			t.Errorf("VersionFromTag(%q, %q) = %q, %v; want %q, %v", tt.format, tt.tag, got, ok, tt.want, tt.ok)
		}
	}
	if got := TagFor("api-v${version}", "1.0.0"); got != "api-v1.0.0" {
		t.Errorf("TagFor = %q", got)
	}
}
