package versioning

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"v5.1.0", "5.1.0", 0, true},
		{"v5.1.0", "v5.0.9", 1, true},
		{"v5.1.0-rc.1", "v5.1.0", -1, true},
		{"v5.1.0-rc.10", "v5.1.0-rc.9", 1, true},
		{"v5.1.0-alpha.3", "v5.1.0-rc.1", -1, true},
		{"v5.1.0+build.7", "v5.1.0", 0, true},
		{"dev", "v5.1.0", 0, false},
		{"v5.1.0", "", 0, false},
	}
	for _, tt := range tests {
		got, ok := Compare(tt.a, tt.b)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d, %v", tt.a, tt.b, got, ok, tt.want, tt.ok)
		}
	}
}

func TestValid(t *testing.T) {
	for s, want := range map[string]bool{"v5.1.0": true, "5.1.0-rc.1": true, "dev": false, "": false, "v5.1": false} {
		if got := Valid(s); got != want {
			t.Errorf("Valid(%q) = %v, want %v", s, got, want)
		}
	}
}
