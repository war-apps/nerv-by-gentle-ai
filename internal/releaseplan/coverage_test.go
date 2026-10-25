package releaseplan

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
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

func TestCoverage_TextNeverRoundsUpToTheThreshold(t *testing.T) {
	tests := []struct {
		name             string
		covered, valid   int
		threshold        float64
		wantErr, wantStr string
	}{
		{"79.996 is below 80", 79996, 100000, 80, "line coverage 79.99% (79996 of 100000 lines, 1 report) is below the threshold 80%", ""},
		{"just under a fractional threshold", 8549, 10000, 85.5, "line coverage 85.49% (8549 of 10000 lines, 1 report) is below the threshold 85.5%", ""},
		{"a pass is truncated too", 99999, 100000, 99, "", "line coverage 99.99% (99999 of 100000 lines, 1 report), threshold 99%"},
		{"exact", 95, 100, 90, "", "line coverage 95.00% (95 of 100 lines, 1 report), threshold 90%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := reports(t, map[string]string{"r/coverage.cobertura.xml": cobertura(tt.covered, tt.valid)})
			c, err := CheckCoverage(dir, tt.threshold)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("CheckCoverage error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.String() != tt.wantStr {
				t.Errorf("String() = %q, want %q", c.String(), tt.wantStr)
			}
		})
	}
}

// coberturaFull is a Cobertura report with branch counts and methods: each
// entry of methods is the hit count of that method's lines.
func coberturaFull(linesCovered, linesValid, branchesCovered, branchesValid int, methods ...[]int) string {
	var m strings.Builder
	for i, hits := range methods {
		m.WriteString(`<method name="M` + itoa(i) + `" signature="()" line-rate="0" branch-rate="0" complexity="1"><lines>`)
		for j, h := range hits {
			m.WriteString(`<line number="` + itoa(j+1) + `" hits="` + itoa(h) + `" branch="False" />`)
		}
		m.WriteString(`</lines></method>`)
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<coverage line-rate="0" branch-rate="0" version="1.9" timestamp="1760000000" lines-covered="` + itoa(linesCovered) + `" lines-valid="` + itoa(linesValid) + `" branches-covered="` + itoa(branchesCovered) + `" branches-valid="` + itoa(branchesValid) + `">
  <packages><package name="Api"><classes><class name="Api.C" filename="C.cs"><methods>` + m.String() + `</methods><lines /></class></classes></package></packages>
</coverage>
`
}

func TestCheckCoverageFloors(t *testing.T) {
	// 2 of 3 methods covered (66.66%), 7 of 10 branches, 9 of 10 lines.
	one := coberturaFull(9, 10, 7, 10, []int{1, 0}, []int{0, 0}, []int{0, 3})
	tests := []struct {
		name    string
		floors  releaseconfig.CoverageThreshold
		files   map[string]string
		want    string
		wantErr string
	}{
		{
			name:   "every floor met",
			floors: releaseconfig.CoverageThreshold{Line: 90, Branch: 70, Method: 66},
			files:  map[string]string{"a/coverage.cobertura.xml": one},
			want:   "line coverage 90.00% (9 of 10 lines, 1 report), threshold 90%; branch coverage 70.00% (7 of 10 branches, 1 report), threshold 70%; method coverage 66.66% (2 of 3 methods, 1 report), threshold 66%",
		},
		{
			name:    "branch below its floor",
			floors:  releaseconfig.CoverageThreshold{Line: 80, Branch: 75},
			files:   map[string]string{"a/coverage.cobertura.xml": one},
			wantErr: "line coverage 90.00% (9 of 10 lines, 1 report), threshold 80%; branch coverage 70.00% (7 of 10 branches, 1 report) is below the threshold 75%",
		},
		{
			name:    "method below its floor, line not checked",
			floors:  releaseconfig.CoverageThreshold{Method: 70},
			files:   map[string]string{"a/coverage.cobertura.xml": one},
			wantErr: "method coverage 66.66% (2 of 3 methods, 1 report) is below the threshold 70%",
		},
		{
			name:   "reports are summed per metric",
			floors: releaseconfig.CoverageThreshold{Branch: 50, Method: 50},
			files: map[string]string{
				"a/coverage.cobertura.xml": coberturaFull(1, 1, 1, 4, []int{0}),
				"b/coverage.cobertura.xml": coberturaFull(1, 1, 3, 4, []int{2}),
			},
			want: "branch coverage 50.00% (4 of 8 branches, 2 reports), threshold 50%; method coverage 50.00% (1 of 2 methods, 2 reports), threshold 50%",
		},
		{
			name:   "nothing to cover is 100%, as coverlet counts it",
			floors: releaseconfig.CoverageThreshold{Branch: 100, Method: 100},
			files:  map[string]string{"a/coverage.cobertura.xml": coberturaFull(0, 0, 0, 0)},
			want:   "branch coverage 100.00% (0 of 0 branches, 1 report), threshold 100%; method coverage 100.00% (0 of 0 methods, 1 report), threshold 100%",
		},
		{
			name:    "a branch floor needs the branch counts",
			floors:  releaseconfig.CoverageThreshold{Branch: 50},
			files:   map[string]string{"a/coverage.cobertura.xml": `<coverage lines-covered="1" lines-valid="1"/>`},
			wantErr: "a/coverage.cobertura.xml: branches-covered / branches-valid missing or invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := CheckCoverageFloors(reports(t, tt.files), tt.floors)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.String() != tt.want {
				t.Errorf("String() = %q, want %q", c.String(), tt.want)
			}
		})
	}
}

// Previous behaviour: a line-only floor reads, and fails, exactly as the
// scalar coverage_threshold always did, even on a report without branch
// counts.
func TestCheckCoverageFloors_LineOnlyIsTheScalarGate(t *testing.T) {
	dir := reports(t, map[string]string{"a/coverage.cobertura.xml": `<coverage lines-covered="799" lines-valid="1000"/>`})
	_, err := CheckCoverageFloors(dir, releaseconfig.CoverageThreshold{Line: 80})
	if want := "line coverage 79.90% (799 of 1000 lines, 1 report) is below the threshold 80%"; err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	_, old := CheckCoverage(dir, 80)
	if old == nil || old.Error() != err.Error() {
		t.Errorf("CheckCoverage = %v, want the same error", old)
	}
}
