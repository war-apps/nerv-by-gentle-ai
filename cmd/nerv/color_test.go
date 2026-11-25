package main

import "testing"

// The wizard is colored only on a terminal that can show colors, and
// never when NO_COLOR is set or TERM is dumb.
func TestColorOutput(t *testing.T) {
	yes := func() bool { return true }
	no := func() bool { return false }
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	cases := []struct {
		name   string
		enable func() bool
		vars   map[string]string
		want   bool
	}{
		{"terminal", yes, nil, true},
		{"not a terminal", no, nil, false},
		{"no detector", nil, nil, false},
		{"NO_COLOR set", yes, map[string]string{"NO_COLOR": "1"}, false},
		{"TERM dumb", yes, map[string]string{"TERM": "dumb"}, false},
	}
	for _, c := range cases {
		opts := options{EnableColor: c.enable, Getenv: env(c.vars)}
		if got := colorOutput(opts); got != c.want {
			t.Errorf("%s: colorOutput = %v, want %v", c.name, got, c.want)
		}
	}
}
