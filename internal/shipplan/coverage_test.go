package shipplan

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// cobertura is a coverlet-style Cobertura report.
func cobertura(covered, valid int) string {
	rate := 0.0
	if valid > 0 {
		rate = float64(covered) / float64(valid)
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<coverage line-rate="` + ftoa(rate) + `" branch-rate="1" version="1.9" timestamp="1760000000" lines-covered="` + itoa(covered) + `" lines-valid="` + itoa(valid) + `" branches-covered="0" branches-valid="0">
  <sources><source>/src/</source></sources>
  <packages />
</coverage>
`
}

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 4, 64) }

// reports writes files (relative path -> content) under a temp dir.
func reports(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCheckCoverage(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		threshold float64
		covered   int
		valid     int
		wantErr   string
	}{
		{
			name:      "one report above the threshold",
			files:     map[string]string{"6f1c/coverage.cobertura.xml": cobertura(90, 100)},
			threshold: 80, covered: 90, valid: 100,
		},
		{
			name:      "exactly the threshold passes",
			files:     map[string]string{"a/coverage.cobertura.xml": cobertura(80, 100)},
			threshold: 80, covered: 80, valid: 100,
		},
		{
			name:      "below the threshold fails",
			files:     map[string]string{"a/coverage.cobertura.xml": cobertura(799, 1000)},
			threshold: 80, covered: 799, valid: 1000,
			wantErr: "line coverage 79.90% (799 of 1000 lines, 1 report) is below the threshold 80%",
		},
		{
			name: "several reports merge by lines, not by averaging rates",
			files: map[string]string{
				"unit/coverage.cobertura.xml":        cobertura(100, 100),
				"integration/coverage.cobertura.xml": cobertura(10, 300),
				"notes.txt":                          "not a report",
			},
			// Average of the rates: (100% + 3.3%) / 2 = 51.7%; by lines: 27.5%.
			threshold: 50, covered: 110, valid: 400,
			wantErr: "line coverage 27.50% (110 of 400 lines, 2 reports) is below the threshold 50%",
		},
		{
			name:      "other *.cobertura.xml names count",
			files:     map[string]string{"web.cobertura.xml": cobertura(5, 10)},
			threshold: 50, covered: 5, valid: 10,
		},
		{
			name:      "no report",
			files:     map[string]string{"x/other.xml": cobertura(1, 1)},
			threshold: 50,
			wantErr:   "no Cobertura report (*.cobertura.xml) under",
		},
		{
			name:      "malformed XML",
			files:     map[string]string{"a/coverage.cobertura.xml": "<coverage lines-covered=\"1\""},
			threshold: 50,
			wantErr:   "a/coverage.cobertura.xml: not a Cobertura report",
		},
		{
			name:      "wrong root element",
			files:     map[string]string{"a/coverage.cobertura.xml": "<report/>"},
			threshold: 50,
			wantErr:   "a/coverage.cobertura.xml: not a Cobertura report (root element <report>)",
		},
		{
			name:      "missing line counts",
			files:     map[string]string{"a/coverage.cobertura.xml": `<coverage line-rate="0.9"></coverage>`},
			threshold: 50,
			wantErr:   "a/coverage.cobertura.xml: lines-covered / lines-valid missing or invalid",
		},
		{
			name:      "covered above valid",
			files:     map[string]string{"a/coverage.cobertura.xml": `<coverage lines-covered="5" lines-valid="4"/>`},
			threshold: 50,
			wantErr:   "lines-covered / lines-valid missing or invalid",
		},
		{
			name:      "no coverable lines",
			files:     map[string]string{"a/coverage.cobertura.xml": cobertura(0, 0)},
			threshold: 50,
			wantErr:   "no coverable lines in 1 report",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := reports(t, tt.files)
			got, err := CheckCoverage(dir, tt.threshold)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(filepath.ToSlash(err.Error()), tt.wantErr) {
					t.Fatalf("CheckCoverage = %v, want an error containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("CheckCoverage = %v", err)
			}
			if tt.valid > 0 && (got.Covered != tt.covered || got.Valid != tt.valid) {
				t.Errorf("coverage = %d/%d, want %d/%d", got.Covered, got.Valid, tt.covered, tt.valid)
			}
		})
	}
}
