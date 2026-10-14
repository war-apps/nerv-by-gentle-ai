package proc

import (
	"strings"
	"testing"
)

func TestTailBuffer_KeepsTheLastBytes(t *testing.T) {
	tests := []struct {
		name   string
		limit  int
		writes []string
		want   string
	}{
		{"under the limit", 10, []string{"abc", "def"}, "abcdef"},
		{"exactly the limit", 6, []string{"abc", "def"}, "abcdef"},
		{"one write over the limit", 4, []string{"abcdefgh"}, "efgh"},
		{"many small writes", 5, strings.Split("0123456789abcdef", ""), "bcdef"},
		{"write after compaction", 3, []string{"abcdefg", "h", "ij"}, "hij"},
		{"empty writes", 3, []string{"", "ab", ""}, "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTailBuffer(tt.limit)
			total := 0
			for _, w := range tt.writes {
				n, err := b.Write([]byte(w))
				if err != nil || n != len(w) {
					t.Fatalf("Write(%q) = %d, %v", w, n, err)
				}
				total += n
				if len(b.buf) > 2*tt.limit {
					t.Fatalf("buffer grew to %d bytes, want at most %d", len(b.buf), 2*tt.limit)
				}
			}
			if got := string(b.Bytes()); got != tt.want {
				t.Errorf("Bytes() = %q, want %q", got, tt.want)
			}
		})
	}
}
