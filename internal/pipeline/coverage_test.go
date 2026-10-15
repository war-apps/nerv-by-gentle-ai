package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// dimRunsettings is dim.comprobantes.api's coverlet.runsettings.
const dimRunsettings = `<?xml version="1.0" encoding="utf-8"?>
<RunSettings>
  <DataCollectionRunSettings>
    <DataCollectors>
      <DataCollector friendlyName="XPlat code coverage">
        <Configuration>
          <Format>cobertura</Format>
          <ExcludeByAttribute>ExcludeFromCodeCoverage,GeneratedCode,CompilerGenerated</ExcludeByAttribute>
          <ExcludeByFile>**/Program.cs,**/AssemblyReference.cs,**/Migrations/*.cs,**/Dtos/**/*.cs,**/Properties/**/*.cs</ExcludeByFile>
          <Exclude>[*.Api]Dim.Comprobantes.Api.Program,[*]*Dto,[*]*Marker,[*]*DependencyInjection</Exclude>
          <Threshold>100,80,100</Threshold>
          <ThresholdType>line,branch,method</ThresholdType>
          <ThresholdStat>total</ThresholdStat>
          <SingleHit>false</SingleHit>
          <UseSourceLink>true</UseSourceLink>
          <DeterministicReport>true</DeterministicReport>
        </Configuration>
      </DataCollector>
    </DataCollectors>
  </DataCollectionRunSettings>
</RunSettings>
`

func collector(config string) string {
	return `<RunSettings><DataCollectionRunSettings><DataCollectors><DataCollector friendlyName="XPlat code coverage"><Configuration>` +
		config + `</Configuration></DataCollector></DataCollectors></DataCollectionRunSettings></RunSettings>`
}

func TestParseThresholds(t *testing.T) {
	tests := []struct {
		name    string
		xml     string
		want    *Thresholds
		wantErr string
	}{
		{"dim's file", dimRunsettings, &Thresholds{Line: ptr(100), Branch: ptr(80), Method: ptr(100), Stat: StatTotal}, ""},
		{"one value for the default types, default statistic", collector(`<Threshold>90</Threshold>`), &Thresholds{Line: ptr(90), Branch: ptr(90), Method: ptr(90), Stat: StatMinimum}, ""},
		{"type subset", collector(`<Threshold>75.5</Threshold><ThresholdType>line</ThresholdType><ThresholdStat>Average</ThresholdStat>`), &Thresholds{Line: ptr(75.5), Stat: StatAverage}, ""},
		{"values in the type order", collector(`<Threshold>70,60</Threshold><ThresholdType>method, line</ThresholdType>`), &Thresholds{Line: ptr(60), Method: ptr(70), Stat: StatMinimum}, ""},
		{"no threshold", collector(`<Format>cobertura</Format>`), nil, ""},
		{"empty threshold", collector(`<Threshold> </Threshold>`), nil, ""},
		{"another collector", strings.Replace(dimRunsettings, "XPlat code coverage", "Code Coverage", 1), nil, ""},
		{"not a number", collector(`<Threshold>abc</Threshold>`), nil, `"abc" is not a percentage`},
		{"out of range", collector(`<Threshold>120</Threshold>`), nil, `"120" is not a percentage`},
		{"count mismatch", collector(`<Threshold>1,2</Threshold>`), nil, "2 values for 3 types"},
		{"unknown type", collector(`<Threshold>1</Threshold><ThresholdType>lines</ThresholdType>`), nil, `unknown ThresholdType "lines"`},
		{"unknown statistic", collector(`<Threshold>1</Threshold><ThresholdStat>median</ThresholdStat>`), nil, `unknown ThresholdStat "median"`},
		{"malformed xml", `<RunSettings><DataCollectionRunSettings>`, nil, "runsettings"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseThresholds([]byte(tt.xml))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (got == nil) != (tt.want == nil) || got != nil && got.String() != tt.want.String() {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func ptr(v float64) *float64 { return &v }

// The probe's model (coverlet.core 6.0.4 called directly): module A has M1
// (lines 1 hit, 2 not; two branches, one hit) and M2 (line 3 not hit);
// module B has M3 (line 10 hit). coverlet computed: line 2/4 = 50 (A 33.33,
// B 100, average 66.66), branch 1/2 = 50 (A 50, B 100 with no branches,
// average 75), method 2/3 = 66.66 (A 50, B 100, average 75).
const reportA = `<?xml version="1.0" encoding="utf-8"?>
<coverage line-rate="0.33" branch-rate="0.5" version="1.9" lines-covered="1" lines-valid="3" branches-covered="1" branches-valid="2">
  <packages>
    <package name="A">
      <classes>
        <class name="A.C" filename="a.cs">
          <methods>
            <method name="M1" signature="()">
              <lines>
                <line number="1" hits="1" branch="True" condition-coverage="50% (1/2)"><conditions><condition number="0" type="jump" coverage="50%" /></conditions></line>
                <line number="2" hits="0" branch="False" />
              </lines>
            </method>
            <method name="M2" signature="()">
              <lines><line number="3" hits="0" branch="False" /></lines>
            </method>
          </methods>
          <lines>
            <line number="1" hits="1" branch="True" condition-coverage="50% (1/2)" />
            <line number="2" hits="0" branch="False" />
            <line number="3" hits="0" branch="False" />
          </lines>
        </class>
      </classes>
    </package>
  </packages>
</coverage>
`

const reportB = `<?xml version="1.0" encoding="utf-8"?>
<coverage line-rate="1" branch-rate="1" version="1.9" lines-covered="1" lines-valid="1" branches-covered="0" branches-valid="0">
  <packages>
    <package name="B">
      <classes>
        <class name="B.D" filename="b.cs">
          <methods>
            <method name="M3" signature="()">
              <lines><line number="10" hits="1" branch="False" /></lines>
            </method>
          </methods>
        </class>
      </classes>
    </package>
  </packages>
</coverage>
`

func writeReports(t *testing.T, reports ...string) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for i, r := range reports {
		p := filepath.Join(dir, string(rune('a'+i)), CoverageFile)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(r), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

func TestMeasure_MatchesCoverlet(t *testing.T) {
	cov, err := readCoverage(writeReports(t, reportA, reportB))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]string{
		StatTotal:   {"line": "50", "branch": "50", "method": "66.66"},
		StatAverage: {"line": "66.66", "branch": "75", "method": "75"},
		StatMinimum: {"line": "33.33", "branch": "50", "method": "50"},
	}
	for stat, metrics := range want {
		for metric, w := range metrics {
			if got := percentText(cov.percent(metric, stat)); got != w {
				t.Errorf("%s %s = %s%%, want %s%% (coverlet)", stat, metric, got, w)
			}
		}
	}
}

func TestMeasure_ReportsAreMergedPerLine(t *testing.T) {
	// A second test project's report covers line 3 of module A (method M2):
	// merged per line as coverlet merges results, A is not counted twice.
	other := strings.Replace(reportA, `<line number="3" hits="0" branch="False" /></lines>`, `<line number="3" hits="2" branch="False" /></lines>`, 1)
	other = strings.Replace(other, `<line number="1" hits="1" branch="True"`, `<line number="1" hits="0" branch="True"`, 1)
	cov, err := readCoverage(writeReports(t, reportA, other))
	if err != nil {
		t.Fatal(err)
	}
	if got := percentText(cov.percent("line", StatTotal)); got != "66.66" {
		t.Errorf("merged line coverage %s%%, want 66.66%% (lines 1 and 3 of 3)", got)
	}
	if got := percentText(cov.percent("method", StatTotal)); got != "100" {
		t.Errorf("merged method coverage %s%%, want 100%%", got)
	}
	if got := percentText(cov.percent("branch", StatTotal)); got != "50" {
		t.Errorf("merged branch coverage %s%%, want 50%%", got)
	}
}

func TestCheckThresholds_EachStatisticAsCoverlet(t *testing.T) {
	cov, err := readCoverage(writeReports(t, reportA, reportB))
	if err != nil {
		t.Fatal(err)
	}
	// The 15 rows of the coverlet.core 6.0.4 probe (GetThresholdTypesBelowThreshold):
	// thresholds -> types below.
	tests := []struct {
		stat                 string
		line, branch, method float64
		below                string
	}{
		{StatMinimum, 50, 50, 66.66, "line,method"},
		{StatMinimum, 50.01, 50, 66.67, "line,method"},
		{StatMinimum, 66.66, 0, 0, "line"},
		{StatMinimum, 33.33, 0, 0, ""},
		{StatMinimum, 33.34, 0, 0, "line"},
		{StatAverage, 50, 50, 66.66, ""},
		{StatAverage, 50.01, 50, 66.67, ""},
		{StatAverage, 66.66, 0, 0, ""},
		{StatAverage, 33.33, 0, 0, ""},
		{StatAverage, 33.34, 0, 0, ""},
		{StatTotal, 50, 50, 66.66, ""},
		{StatTotal, 50.01, 50, 66.67, "line,method"},
		{StatTotal, 66.66, 0, 0, "line"},
		{StatTotal, 33.33, 0, 0, ""},
		{StatTotal, 33.34, 0, 0, ""},
	}
	for _, tt := range tests {
		th := &Thresholds{Line: ptr(tt.line), Branch: ptr(tt.branch), Method: ptr(tt.method), Stat: tt.stat}
		results := checkThresholds(cov, th)
		var below []string
		for _, r := range results {
			if !r.Met {
				below = append(below, r.Type)
			}
		}
		if got := strings.Join(below, ","); got != tt.below {
			t.Errorf("%s %v/%v/%v: below %q, want %q (coverlet)", tt.stat, tt.line, tt.branch, tt.method, got, tt.below)
		}
	}
}

func TestPercentText_TruncatesNeverRoundsUp(t *testing.T) {
	for hundredths, want := range map[int]string{10000: "100", 9999: "99.99", 8571: "85.71", 5000: "50", 5010: "50.1", 0: "0", 1: "0.01"} {
		if got := percentText(hundredths); got != want {
			t.Errorf("percentText(%d) = %q, want %q", hundredths, got, want)
		}
	}
	// 6/7 = 85.714...% -> 85.71, and 199/200 = 99.5 never 100.
	if got := percentText(hundredthsOf(6, 7)); got != "85.71" {
		t.Errorf("6/7 = %s", got)
	}
	if got := percentText(hundredthsOf(19999, 20000)); got != "99.99" {
		t.Errorf("19999/20000 = %s", got)
	}
}

func TestTest_EnforcesTheRunsettingsThresholds(t *testing.T) {
	tests := []struct {
		name, settings string
		reports        []string
		status, text   string
	}{
		{"met", collector(`<Threshold>50,50,66</Threshold><ThresholdStat>total</ThresholdStat>`), []string{reportA, reportB}, StatusPassed,
			"coverage (total): line 50% >= 50%, branch 50% >= 50%, method 66.66% >= 66%"},
		{"not met", dimRunsettings, []string{reportA, reportB}, StatusFailed,
			"coverage below the thresholds of coverlet.runsettings (total): line 50% < 100%, branch 50% < 80%, method 66.66% < 100%"},
		{"no report", dimRunsettings, nil, StatusFailed, "no coverage report"},
		{"no threshold: not checked", collector(`<Format>cobertura</Format>`), []string{reportA}, StatusPassed, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			if err := os.WriteFile(filepath.Join(repo, "coverlet.runsettings"), []byte(tt.settings), 0o644); err != nil {
				t.Fatal(err)
			}
			results := ResultsDir(repo)
			run := &writingSteps{inner: &fakeSteps{}, write: func() {
				for i, r := range tt.reports {
					p := filepath.Join(results, string(rune('a'+i)), CoverageFile)
					os.MkdirAll(filepath.Dir(p), 0o755)
					os.WriteFile(p, []byte(r), 0o644)
				}
			}}
			cfg := config(nil)
			cfg.Dotnet.Runsettings = "coverlet.runsettings"
			r := Steps{Repo: repo, Config: cfg, Run: run}.Test(context.Background())
			if r.Status != tt.status {
				t.Fatalf("status %s, want %s: %+v", r.Status, tt.status, r)
			}
			text := r.Error
			if tt.status == StatusPassed && len(r.Notes) > 0 {
				text = r.Notes[0]
			}
			if tt.text == "" && (len(r.Notes) != 0 || r.Error != "") || tt.text != "" && !strings.Contains(text, tt.text) {
				t.Errorf("text %q (notes %v), want %q", text, r.Notes, tt.text)
			}
		})
	}
}

func TestMeasure_NoModulesIsFullCoverage(t *testing.T) {
	empty := `<?xml version="1.0"?><coverage><packages /></coverage>`
	cov, err := readCoverage(writeReports(t, empty))
	if err != nil {
		t.Fatal(err)
	}
	for _, stat := range []string{StatTotal, StatAverage, StatMinimum} {
		if got := cov.percent("line", stat); got != 10000 {
			t.Errorf("%s: %d hundredths, want 10000 (nothing to cover)", stat, got)
		}
	}
}

// jsonReport is coverlet's JSON format: module -> document -> class ->
// method -> {Lines, Branches}.
func jsonReport(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type jm = map[string]any

func branch(line, offset, end, path, ordinal, hits int) jm {
	return jm{"Line": line, "Offset": offset, "EndOffset": end, "Path": path, "Ordinal": ordinal, "Hits": hits}
}

func writeJSONReports(t *testing.T, reports ...string) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for i, r := range reports {
		p := filepath.Join(dir, string(rune('a'+i)), CoverageJSONFile)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(r), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

func TestMeasureJSON_TwoProjectsCoveringDifferentBranchesOfALine(t *testing.T) {
	// Each test project covers one branch of Sign's ternary (line 4): coverlet
	// merges per branch, 2/2.
	sign := func(pos, neg int) string {
		return jsonReport(t, jm{"Calc.dll": jm{"/src/Calc/Ops.cs": jm{"Calc.Ops": jm{
			"System.Int32 Calc.Ops::Sign(System.Int32)": jm{
				"Lines":    jm{"4": pos + neg},
				"Branches": []jm{branch(4, 2, 4, 0, 0, neg), branch(4, 2, 6, 1, 1, pos)},
			},
		}}}})
	}
	one, err := readJSONCoverage(writeJSONReports(t, sign(1, 0)))
	if err != nil {
		t.Fatal(err)
	}
	if got := percentText(one.percent("branch", StatTotal)); got != "50" {
		t.Errorf("one project: branch %s%%, want 50%%", got)
	}
	both, err := readJSONCoverage(writeJSONReports(t, sign(1, 0), sign(0, 1)))
	if err != nil {
		t.Fatal(err)
	}
	for metric, want := range map[string]string{"line": "100", "branch": "100", "method": "100"} {
		if got := percentText(both.percent(metric, StatTotal)); got != want {
			t.Errorf("merged %s %s%%, want %s%% (coverlet merges per branch)", metric, got, want)
		}
	}
}

func TestMeasureJSON_MatchesCoverlet(t *testing.T) {
	// The probe model of TestMeasure_MatchesCoverlet in coverlet's JSON format.
	a := jsonReport(t, jm{"A.dll": jm{"a.cs": jm{"A.C": jm{
		"M1": jm{"Lines": jm{"1": 1, "2": 0}, "Branches": []jm{branch(1, 0, 1, 0, 0, 1), branch(1, 0, 2, 1, 1, 0)}},
		"M2": jm{"Lines": jm{"3": 0}, "Branches": []jm{}},
	}}}})
	b := jsonReport(t, jm{"B.dll": jm{"b.cs": jm{"B.D": jm{"M3": jm{"Lines": jm{"10": 1}, "Branches": []jm{}}}}}})
	cov, err := readJSONCoverage(writeJSONReports(t, a, b))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]string{
		StatTotal:   {"line": "50", "branch": "50", "method": "66.66"},
		StatAverage: {"line": "66.66", "branch": "75", "method": "75"},
		StatMinimum: {"line": "33.33", "branch": "50", "method": "50"},
	}
	for stat, metrics := range want {
		for metric, w := range metrics {
			if got := percentText(cov.percent(metric, stat)); got != w {
				t.Errorf("%s %s = %s%%, want %s%% (coverlet)", stat, metric, got, w)
			}
		}
	}
}

func TestCollectArg(t *testing.T) {
	tests := []struct {
		name, settings, want string
	}{
		{"no threshold: plain collector", collector(`<Format>cobertura</Format>`), "XPlat Code Coverage"},
		{"threshold, dim's cobertura", dimRunsettings, "XPlat Code Coverage;Format=cobertura,json"},
		{"threshold, no format", collector(`<Threshold>90</Threshold>`), "XPlat Code Coverage;Format=cobertura,json"},
		{"threshold, user formats kept", collector(`<Format>opencover, cobertura</Format><Threshold>90</Threshold>`), "XPlat Code Coverage;Format=opencover,cobertura,json"},
		{"threshold, json already", collector(`<Format>json</Format><Threshold>90</Threshold>`), "XPlat Code Coverage;Format=json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate, err := readGate([]byte(tt.settings))
			if err != nil {
				t.Fatal(err)
			}
			if got := gate.collectArg(); got != tt.want {
				t.Errorf("collectArg = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTest_PrefersTheJSONReports(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "coverlet.runsettings"), []byte(collector(`<Threshold>100</Threshold><ThresholdType>branch</ThresholdType><ThresholdStat>total</ThresholdStat>`)), 0o644); err != nil {
		t.Fatal(err)
	}
	results := ResultsDir(repo)
	sign := func(pos, neg int) string {
		return jsonReport(t, jm{"Calc.dll": jm{"/src/Ops.cs": jm{"Calc.Ops": jm{"Sign": jm{
			"Lines": jm{"4": 1}, "Branches": []jm{branch(4, 2, 4, 0, 0, neg), branch(4, 2, 6, 1, 1, pos)},
		}}}}})
	}
	f := &fakeSteps{}
	run := &writingSteps{inner: f, write: func() {
		// Cobertura says 1/2 per line in each project; the JSON knows which.
		half := strings.Replace(reportA, `<method name="M2"`, `<method name="Mx"`, 1)
		for i, j := range []string{sign(1, 0), sign(0, 1)} {
			dir := filepath.Join(results, string(rune('a'+i)))
			os.MkdirAll(dir, 0o755)
			os.WriteFile(filepath.Join(dir, CoverageJSONFile), []byte(j), 0o644)
			os.WriteFile(filepath.Join(dir, CoverageFile), []byte(half), 0o644)
		}
	}}
	cfg := config(nil)
	cfg.Dotnet.Runsettings = "coverlet.runsettings"
	r := Steps{Repo: repo, Config: cfg, Run: run}.Test(context.Background())
	if r.Status != StatusPassed || len(r.Notes) != 1 || r.Notes[0] != "coverage (total): branch 100% >= 100%" {
		t.Fatalf("result %+v", r)
	}
	if len(r.Artifacts.CoverageJSON) != 2 || len(r.Artifacts.Coverage) != 2 {
		t.Errorf("artifacts %+v", r.Artifacts)
	}
	last := f.steps[len(f.steps)-1].Exec
	if !slices.Contains(last, "XPlat Code Coverage;Format=cobertura,json") {
		t.Errorf("dotnet test argv %v: want the JSON format requested", last)
	}
}

// runReports writes per-run reports: each entry is a run directory holding
// a JSON report, a Cobertura report, or both ("" = absent).
func runReports(t *testing.T, runs ...[2]string) (jsons, cobs []string) {
	t.Helper()
	dir := t.TempDir()
	for i, r := range runs {
		run := filepath.Join(dir, string(rune('a'+i)))
		if err := os.MkdirAll(run, 0o755); err != nil {
			t.Fatal(err)
		}
		if r[0] != "" {
			p := filepath.Join(run, CoverageJSONFile)
			os.WriteFile(p, []byte(r[0]), 0o644)
			jsons = append(jsons, p)
		}
		if r[1] != "" {
			p := filepath.Join(run, CoverageFile)
			os.WriteFile(p, []byte(r[1]), 0o644)
			cobs = append(cobs, p)
		}
	}
	return jsons, cobs
}

func TestCheckCoverage_ChoosesTheReportPerRun(t *testing.T) {
	// Run a has coverlet's JSON for module B (fully covered); run b only a
	// Cobertura report for module A (line 1 of 3 covered). Both count.
	b := jsonReport(t, jm{"B.dll": jm{"b.cs": jm{"B.D": jm{"System.Int32 B.D::M3()": jm{"Lines": jm{"10": 1}, "Branches": []jm{}}}}}})
	jsons, cobs := runReports(t, [2]string{b, ""}, [2]string{"", reportA})
	th := &Thresholds{Line: ptr(100), Stat: StatTotal}
	_, err := checkCoverage("x.runsettings", th, jsons, cobs)
	// lines: A 1/3 + B 1/1 = 2/4.
	if err == nil || !strings.Contains(err.Error(), "line 50% < 100%") {
		t.Fatalf("err = %v, want both runs counted (line 50%%)", err)
	}
}

func TestCheckCoverage_SameModuleFromJSONAndCoberturaIsOneModule(t *testing.T) {
	// The same method of the same assembly, one run in each format: Cobertura
	// package "Calc" is JSON module "Calc.dll"; the method is matched by
	// class, name and signature.
	j := jsonReport(t, jm{"Calc.dll": jm{"/src/Calc/Ops.cs": jm{"Calc.Ops": jm{"System.Int32 Calc.Ops::Add(System.Int32,System.Int32)": jm{"Lines": jm{"4": 1}, "Branches": []jm{}}}}}})
	c := `<?xml version="1.0"?><coverage><packages><package name="Calc"><classes><class name="Calc.Ops" filename="Calc/Ops.cs"><methods>
<method name="Add" signature="(System.Int32,System.Int32)"><lines><line number="4" hits="0" branch="False" /></lines></method>
</methods></class></classes></package></packages></coverage>`
	jsons, cobs := runReports(t, [2]string{j, ""}, [2]string{"", c})
	note, err := checkCoverage("x.runsettings", &Thresholds{Line: ptr(100), Method: ptr(100), Stat: StatMinimum}, jsons, cobs)
	if err != nil || !strings.Contains(note, "line 100% >= 100%, method 100% >= 100%") {
		t.Errorf("note %q, err %v: want one module, one method, covered", note, err)
	}
}

func TestCheckCoverage_UnreadableRunIsAnError(t *testing.T) {
	good := jsonReport(t, jm{"B.dll": jm{"b.cs": jm{"B.D": jm{"M": jm{"Lines": jm{"1": 1}, "Branches": []jm{}}}}}})
	// A broken JSON with a readable Cobertura falls back to the Cobertura.
	jsons, cobs := runReports(t, [2]string{good, ""}, [2]string{"{broken", reportB})
	if _, err := checkCoverage("x.runsettings", &Thresholds{Line: ptr(100), Stat: StatTotal}, jsons, cobs); err != nil {
		t.Errorf("fallback to the run's Cobertura: %v", err)
	}
	// Neither readable: the run is named.
	jsons, cobs = runReports(t, [2]string{good, ""}, [2]string{"{broken", "<coverage><packages"})
	_, err := checkCoverage("x.runsettings", &Thresholds{Line: ptr(100), Stat: StatTotal}, jsons, cobs)
	if err == nil || !strings.Contains(err.Error(), "no readable coverage report in run") || !strings.Contains(err.Error(), filepath.Dir(cobs[0])) {
		t.Errorf("err = %v, want the unreadable run named", err)
	}
}
