package pipeline

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The coverage gate. coverlet's collector ("XPlat Code Coverage", the one
// dim uses) writes the reports but ignores <Threshold> in the runsettings
// (observed with coverlet.collector 6.0.4: 50% line coverage against 100%,
// dotnet test exit 0), so `nerv pipeline test` enforces it, with coverlet's
// own semantics, verified by calling coverlet.core 6.0.4
// (CoverageSummary, CoverageDetails, CoverageResult.GetThresholdTypesBelowThreshold)
// on a two-module model:
//
//   - a percentage is floored to hundredths (2/3 -> 66.66), and a metric
//     with nothing to cover is 100%;
//   - line: covered lines / lines of every method;
//   - branch: covered branches / branches;
//   - method: methods with at least one covered line / methods;
//   - ThresholdStat total: over all modules together; average: the floored
//     mean of the modules' percentages; minimum (coverlet's default): the
//     lowest module's;
//   - a threshold is missed when the percentage is lower than it.
//
// The metrics come from coverlet's JSON report, which the test step asks
// the collector for (in addition to the runsettings' own formats) whenever
// a threshold is checked: it holds every module, document, class, method,
// line hit count and each branch with its own hit count. Several reports
// (one per test project) are merged as coverlet merges results: line hits
// and branch hits added per line and per branch, so a module present in two
// reports is not counted twice and two projects covering different
// branches of one line make 2/2. The collector's Include/Exclude filters
// were already applied when it wrote the reports, so nothing is filtered
// again here.
//
// Without a JSON report (only Cobertura), the Cobertura reports are read
// instead: their packages are the modules, and a line's branches are only
// a count ("50% (1/2)"), not which branches, so the best count of a line
// is kept; that can undercount branches covered by different projects.

// ThresholdStat values.
const (
	StatTotal   = "total"
	StatAverage = "average"
	StatMinimum = "minimum"
)

// coverageMetrics are coverlet's threshold types in its default order.
var coverageMetrics = []string{"line", "branch", "method"}

// Thresholds are the coverage thresholds of a runsettings file; a nil
// metric is not checked.
type Thresholds struct {
	Line, Branch, Method *float64
	Stat                 string
	// source says where each metric's threshold comes from (runsettings
	// when absent).
	source map[string]string
}

// Threshold sources.
const (
	SourceRunsettings = "runsettings"
	SourceConfig      = "nerv.yaml"
)

func (t *Thresholds) sourceOf(metric string) string {
	if s := t.source[metric]; s != "" {
		return s
	}
	return SourceRunsettings
}

// origin names where the thresholds come from, for messages: the
// runsettings file, nerv.yaml or both.
func (t *Thresholds) origin(runsettings string) string {
	file, conf := false, false
	for _, m := range coverageMetrics {
		if t.required(m) == nil {
			continue
		}
		if t.sourceOf(m) == SourceConfig {
			conf = true
		} else {
			file = true
		}
	}
	if runsettings == "" {
		runsettings = "the runsettings"
	}
	switch {
	case file && conf:
		return runsettings + " and " + SourceConfig
	case conf:
		return SourceConfig
	}
	return runsettings
}

// CoverageResult is the test step's coverage gate as run.
type CoverageResult struct {
	// Mode is pipeline.dotnet.coverage.mode: enforce, report or off.
	Mode string `json:"mode"`
	// Stat is coverlet's ThresholdStat applied (empty when nothing was
	// checked).
	Stat string `json:"stat,omitempty"`
	// Checks are the thresholds checked, with the measured values and each
	// threshold's source.
	Checks []MetricCheck `json:"checks,omitempty"`
	// Met: every checked threshold met (true when none was checked; false
	// when the reports could not be read).
	Met bool `json:"met"`
	// ReportMissing: thresholds were set but no readable coverage report
	// was found, so nothing was measured (Checks is empty).
	ReportMissing bool `json:"report_missing,omitempty"`
}

// effectiveGate returns the gate the test step runs: g's report formats
// and the runsettings' thresholds (g may be nil), overridden key by key by
// c.Thresholds; no thresholds with mode off. nil when nothing is
// configured at all.
func effectiveGate(g *gate, c pipelineconfig.Coverage, mode string) *gate {
	out := &gate{formats: []string{"cobertura"}}
	if g != nil {
		out.formats = g.formats
	}
	if mode == "off" {
		if g == nil {
			return nil
		}
		return out
	}
	var th *Thresholds
	if g != nil && g.th != nil {
		cp := *g.th
		th = &cp
	}
	// An override without a metric only changes the stat of existing
	// thresholds: it never arms an empty gate.
	if o := c.Thresholds; o != nil && (th != nil || o.Line != nil || o.Branch != nil || o.Method != nil) {
		if th == nil {
			th = &Thresholds{Stat: StatMinimum}
		}
		th.source = map[string]string{}
		for _, m := range []struct {
			name string
			v    *float64
			dst  **float64
		}{{"line", o.Line, &th.Line}, {"branch", o.Branch, &th.Branch}, {"method", o.Method, &th.Method}} {
			if m.v != nil {
				v := *m.v
				*m.dst = &v
				th.source[m.name] = SourceConfig
			}
		}
		if o.Stat != "" {
			th.Stat = o.Stat
		}
	}
	out.th = th
	if g == nil && th == nil {
		return nil
	}
	return out
}

// gateCoverage checks th on the step's reports as mode says: enforce fails
// the step below a threshold (or without a report), report records and
// warns, and r.Coverage holds the checks either way.
func gateCoverage(r *Result, runsettings, mode string, th *Thresholds) {
	checks, err := measureCoverage(runsettings, th, r.Artifacts.CoverageJSON, r.Artifacts.Coverage)
	enforce := mode != "report"
	if err != nil {
		r.Coverage.Met, r.Coverage.ReportMissing = false, true
		if enforce {
			r.fail(err)
		} else {
			r.Notes = append(r.Notes, "warning: "+err.Error()+", not enforced (coverage.mode report)")
		}
		return
	}
	r.Coverage.Stat, r.Coverage.Checks = th.Stat, checks
	parts := make([]string, len(checks))
	for i, c := range checks {
		parts[i] = c.String()
		r.Coverage.Met = r.Coverage.Met && c.Met
	}
	summary := strings.Join(parts, ", ")
	switch {
	case r.Coverage.Met:
		r.Notes = append(r.Notes, fmt.Sprintf("coverage (%s): %s", th.Stat, summary))
	case enforce:
		r.fail(fmt.Errorf("coverage below the thresholds of %s (%s): %s", th.origin(runsettings), th.Stat, summary))
	default:
		r.Notes = append(r.Notes, fmt.Sprintf("warning: coverage below the thresholds of %s (%s), not enforced (coverage.mode report): %s", th.origin(runsettings), th.Stat, summary))
	}
}

func (t *Thresholds) required(metric string) *float64 {
	switch metric {
	case "line":
		return t.Line
	case "branch":
		return t.Branch
	}
	return t.Method
}

func (t *Thresholds) String() string {
	var parts []string
	for _, m := range coverageMetrics {
		v := "-"
		if r := t.required(m); r != nil {
			v = strconv.FormatFloat(*r, 'f', -1, 64)
		}
		parts = append(parts, m+"="+v)
	}
	return strings.Join(parts, " ") + " stat=" + t.Stat
}

type runSettings struct {
	Collectors []struct {
		FriendlyName string          `xml:"friendlyName,attr"`
		Config       collectorConfig `xml:"Configuration"`
	} `xml:"DataCollectionRunSettings>DataCollectors>DataCollector"`
}

type collectorConfig struct {
	Format        string `xml:"Format"`
	Threshold     string `xml:"Threshold"`
	ThresholdType string `xml:"ThresholdType"`
	ThresholdStat string `xml:"ThresholdStat"`
}

// collectorName is coverlet's collector, as --collect and the runsettings
// name it.
const collectorName = "XPlat Code Coverage"

// gate is the coverage gate of a runsettings file: its thresholds (nil when
// there is none to check) and the collector's report formats.
type gate struct {
	th      *Thresholds
	formats []string
}

// readGate reads coverlet's collector configuration in a runsettings file.
func readGate(data []byte) (*gate, error) {
	var rs runSettings
	if err := xml.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("runsettings: %w", err)
	}
	g := &gate{formats: []string{"cobertura"}}
	for _, c := range rs.Collectors {
		if !strings.EqualFold(strings.TrimSpace(c.FriendlyName), collectorName) {
			continue
		}
		if f := strings.TrimSpace(c.Config.Format); f != "" {
			g.formats = nil
			for _, part := range strings.Split(f, ",") {
				if part = strings.ToLower(strings.TrimSpace(part)); part != "" && !slices.Contains(g.formats, part) {
					g.formats = append(g.formats, part)
				}
			}
		}
		th, err := thresholdsOf(c.Config)
		if err != nil {
			return nil, err
		}
		g.th = th
		break
	}
	return g, nil
}

// collectArg is dotnet test's --collect value: the collector alone, or,
// when a threshold is checked, the collector with its formats plus json,
// which VSTest merges with the runsettings' configuration (verified with
// the .NET 10 SDK: the file's Exclude still applies and both reports are
// written; the "-- DataCollectionRunSettings...Format=..." override is
// rejected together with --settings).
func (g *gate) collectArg() string {
	if g == nil || g.th == nil {
		return collectorName
	}
	formats := g.formats
	if !slices.Contains(formats, "json") {
		formats = append(slices.Clone(formats), "json")
	}
	return collectorName + ";Format=" + strings.Join(formats, ",")
}

// parseThresholds reads the thresholds of coverlet's collector in a
// runsettings file. nil when the file declares no threshold.
func parseThresholds(data []byte) (*Thresholds, error) {
	g, err := readGate(data)
	if err != nil {
		return nil, err
	}
	return g.th, nil
}

// thresholdsOf reads Threshold (comma-separated percentages, in the order
// of ThresholdType, default line,branch,method; a single value applies to
// every type) and ThresholdStat (total, average or minimum; default
// minimum, as coverlet). nil when there is no threshold.
func thresholdsOf(c collectorConfig) (*Thresholds, error) {
	raw := strings.TrimSpace(c.Threshold)
	if raw == "" {
		return nil, nil
	}
	types := coverageMetrics
	if tt := strings.TrimSpace(c.ThresholdType); tt != "" {
		types = nil
		for _, t := range strings.Split(tt, ",") {
			t = strings.ToLower(strings.TrimSpace(t))
			if !slices.Contains(coverageMetrics, t) {
				return nil, fmt.Errorf("runsettings: unknown ThresholdType %q (want line, branch or method)", t)
			}
			types = append(types, t)
		}
	}
	values := strings.Split(raw, ",")
	if len(values) != 1 && len(values) != len(types) {
		return nil, fmt.Errorf("runsettings: Threshold has %d values for %d types (%s)", len(values), len(types), strings.Join(types, ","))
	}
	th := &Thresholds{Stat: StatMinimum}
	if s := strings.ToLower(strings.TrimSpace(c.ThresholdStat)); s != "" {
		if s != StatTotal && s != StatAverage && s != StatMinimum {
			return nil, fmt.Errorf("runsettings: unknown ThresholdStat %q (want total, average or minimum)", s)
		}
		th.Stat = s
	}
	for i, typ := range types {
		text := strings.TrimSpace(values[min(i, len(values)-1)])
		v, err := strconv.ParseFloat(text, 64)
		if err != nil || v < 0 || v > 100 {
			return nil, fmt.Errorf("runsettings: Threshold %q is not a percentage (0 to 100)", text)
		}
		switch typ {
		case "line":
			th.Line = &v
		case "branch":
			th.Branch = &v
		default:
			th.Method = &v
		}
	}
	return th, nil
}

// coverage is the merged coverage of a run, per module.
type coverage struct {
	modules map[string]*covModule
	order   []string
}

type covModule struct {
	methods map[string]*covMethod
}

type covMethod struct {
	hits map[int]int
	// branchHits: hit count per branch (JSON reports).
	branchHits map[branchKey]int
	// branches: covered and total per line (Cobertura fallback).
	branches map[int][2]int
}

// branchKey identifies one branch as coverlet does.
type branchKey struct {
	Line, Offset, EndOffset, Path int
	Ordinal                       uint32
}

func newMethod() *covMethod {
	return &covMethod{hits: map[int]int{}, branchHits: map[branchKey]int{}, branches: map[int][2]int{}}
}

// module returns the module name's entry, created in order.
func (c *coverage) module(name string) *covModule {
	mod := c.modules[name]
	if mod == nil {
		mod = &covModule{methods: map[string]*covMethod{}}
		c.modules[name] = mod
		c.order = append(c.order, name)
	}
	return mod
}

// jsonMethod is one method of coverlet's JSON report.
type jsonMethod struct {
	Lines    map[string]int `json:"Lines"`
	Branches []struct {
		Line, Offset, EndOffset, Path int
		Ordinal                       uint32
		Hits                          int
	} `json:"Branches"`
}

// readJSONCoverage reads and merges coverlet's JSON reports.
func readJSONCoverage(paths []string) (*coverage, error) {
	cov := newCoverage()
	for _, p := range paths {
		if err := cov.addJSON(p); err != nil {
			return nil, err
		}
	}
	return cov, nil
}

func newCoverage() *coverage { return &coverage{modules: map[string]*covModule{}} }

// moduleName is the module of a JSON report key ("Calc.dll") as Cobertura
// names its package ("Calc").
func moduleName(key string) string {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".dll", ".exe":
		return strings.TrimSuffix(key, filepath.Ext(key))
	}
	return key
}

// methodKey identifies a method in both formats: its class, then its name
// and signature (JSON "System.Int32 Calc.Ops::Add(System.Int32)" gives
// "Add(System.Int32)", as Cobertura's name plus signature).
func methodKey(class, method string) string {
	if _, after, ok := strings.Cut(method, "::"); ok {
		method = after
	}
	return class + "\x00" + method
}

// addJSON merges one coverlet JSON report (module -> document -> class ->
// method), adding line and branch hits. The report is read whole first: a
// bad file merges nothing.
func (c *coverage) addJSON(p string) error {
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var rep map[string]map[string]map[string]map[string]jsonMethod
	if err := json.Unmarshal(data, &rep); err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	one := newCoverage()
	for _, modKey := range slices.Sorted(maps.Keys(rep)) {
		mod := one.module(moduleName(modKey))
		for _, classes := range rep[modKey] {
			for cls, methods := range classes {
				for name, m := range methods {
					meth := mod.method(methodKey(cls, name))
					for line, hits := range m.Lines {
						n, err := strconv.Atoi(line)
						if err != nil {
							return fmt.Errorf("%s: line %q of %s", p, line, name)
						}
						meth.hits[n] += hits
					}
					for _, b := range m.Branches {
						meth.branchHits[branchKey{b.Line, b.Offset, b.EndOffset, b.Path, b.Ordinal}] += b.Hits
					}
				}
			}
		}
	}
	c.merge(one)
	return nil
}

// method returns the method's entry, created when missing.
func (m *covModule) method(key string) *covMethod {
	meth := m.methods[key]
	if meth == nil {
		meth = newMethod()
		m.methods[key] = meth
	}
	return meth
}

// merge adds other into c, as coverlet merges results: line and branch hits
// added; Cobertura's per-line branch counts keep the best of a line.
func (c *coverage) merge(other *coverage) {
	for _, name := range other.order {
		mod := c.module(name)
		for key, om := range other.modules[name].methods {
			meth := mod.method(key)
			for line, h := range om.hits {
				meth.hits[line] += h
			}
			for b, h := range om.branchHits {
				meth.branchHits[b] += h
			}
			for line, b := range om.branches {
				prev := meth.branches[line]
				meth.branches[line] = [2]int{max(prev[0], b[0]), max(prev[1], b[1])}
			}
		}
	}
}

type coberturaReport struct {
	Packages []struct {
		Name    string `xml:"name,attr"`
		Classes []struct {
			Name     string `xml:"name,attr"`
			Filename string `xml:"filename,attr"`
			Methods  []struct {
				Name      string        `xml:"name,attr"`
				Signature string        `xml:"signature,attr"`
				Lines     []coberturaLn `xml:"lines>line"`
			} `xml:"methods>method"`
		} `xml:"classes>class"`
	} `xml:"packages>package"`
}

type coberturaLn struct {
	Number    int    `xml:"number,attr"`
	Hits      int    `xml:"hits,attr"`
	Branch    string `xml:"branch,attr"`
	Condition string `xml:"condition-coverage,attr"`
}

var conditionRe = regexp.MustCompile(`\((\d+)/(\d+)\)`)

// readCoverage reads and merges coverlet's Cobertura reports.
func readCoverage(paths []string) (*coverage, error) {
	cov := newCoverage()
	for _, p := range paths {
		if err := cov.addCobertura(p); err != nil {
			return nil, err
		}
	}
	return cov, nil
}

// addCobertura merges one Cobertura report: line hits, and per line only
// the count of covered branches (Cobertura does not say which), so the
// best count of a line is kept. The report is read whole first: a bad file
// merges nothing.
func (c *coverage) addCobertura(p string) error {
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var rep coberturaReport
	if err := xml.Unmarshal(data, &rep); err != nil {
		return fmt.Errorf("%s: %w", p, err)
	}
	one := newCoverage()
	for _, pkg := range rep.Packages {
		mod := one.module(pkg.Name)
		for _, cls := range pkg.Classes {
			for _, m := range cls.Methods {
				meth := mod.method(methodKey(cls.Name, m.Name+m.Signature))
				for _, ln := range m.Lines {
					meth.hits[ln.Number] += ln.Hits
					if !strings.EqualFold(ln.Branch, "true") {
						continue
					}
					match := conditionRe.FindStringSubmatch(ln.Condition)
					if match == nil {
						return fmt.Errorf("%s: line %d: unreadable condition-coverage %q", p, ln.Number, ln.Condition)
					}
					covered, _ := strconv.Atoi(match[1])
					total, _ := strconv.Atoi(match[2])
					prev := meth.branches[ln.Number]
					meth.branches[ln.Number] = [2]int{max(prev[0], covered), max(prev[1], total)}
				}
			}
		}
	}
	c.merge(one)
	return nil
}

// counts returns a module's covered and total for metric.
func (m *covModule) counts(metric string) (covered, total int) {
	for _, meth := range m.methods {
		anyHit := false
		for _, h := range meth.hits {
			if h > 0 {
				anyHit = true
			}
		}
		switch metric {
		case "line":
			for _, h := range meth.hits {
				total++
				if h > 0 {
					covered++
				}
			}
		case "branch":
			// Per line: the JSON branches (each known), with a Cobertura
			// count for the same line as a floor (another run that only
			// wrote Cobertura); a line known only from Cobertura adds its
			// count.
			perLine := map[int][2]int{}
			for b, h := range meth.branchHits {
				v := perLine[b.Line]
				v[1]++
				if h > 0 {
					v[0]++
				}
				perLine[b.Line] = v
			}
			for line, b := range meth.branches {
				v, known := perLine[line]
				if !known {
					perLine[line] = b
					continue
				}
				v[0] = min(max(v[0], b[0]), v[1])
				perLine[line] = v
			}
			for _, v := range perLine {
				covered += v[0]
				total += v[1]
			}
		default:
			total++
			if anyHit {
				covered++
			}
		}
	}
	return covered, total
}

// hundredthsOf is covered/total as a percentage in hundredths, floored;
// nothing to cover is 100%.
func hundredthsOf(covered, total int) int {
	if total == 0 {
		return 10000
	}
	return int(int64(covered) * 10000 / int64(total))
}

// percent is metric's percentage in hundredths under stat.
func (c *coverage) percent(metric, stat string) int {
	if len(c.order) == 0 {
		// No module: nothing to cover, 100% (the rule of an empty metric).
		return 10000
	}
	var covered, total, sum int
	lowest := 10000
	for _, name := range c.order {
		mc, mt := c.modules[name].counts(metric)
		covered, total = covered+mc, total+mt
		h := hundredthsOf(mc, mt)
		sum += h
		lowest = min(lowest, h)
	}
	switch stat {
	case StatAverage:
		return sum / len(c.order)
	case StatMinimum:
		return lowest
	}
	return hundredthsOf(covered, total)
}

// percentText formats hundredths of a percent without trailing zeros.
func percentText(hundredths int) string {
	s := fmt.Sprintf("%d.%02d", hundredths/100, hundredths%100)
	return strings.TrimSuffix(strings.TrimRight(s, "0"), ".")
}

// MetricCheck is one threshold checked.
type MetricCheck struct {
	Type     string  `json:"type"`
	Actual   string  `json:"actual"`
	Required float64 `json:"required"`
	// Source is where the threshold comes from: runsettings or nerv.yaml.
	Source string `json:"source"`
	Met    bool   `json:"met"`
}

func (m MetricCheck) String() string {
	op := ">="
	if !m.Met {
		op = "<"
	}
	return fmt.Sprintf("%s %s%% %s %s%%", m.Type, m.Actual, op, strconv.FormatFloat(m.Required, 'f', -1, 64))
}

// checkThresholds compares the coverage with every set threshold, in
// coverlet's type order.
func checkThresholds(cov *coverage, th *Thresholds) []MetricCheck {
	var out []MetricCheck
	for _, metric := range coverageMetrics {
		req := th.required(metric)
		if req == nil {
			continue
		}
		h := cov.percent(metric, th.Stat)
		out = append(out, MetricCheck{Type: metric, Actual: percentText(h), Required: *req, Source: th.sourceOf(metric), Met: !(float64(h)/100 < *req)})
	}
	return out
}

// checkCoverage enforces th on the run's reports. Each test run (one per
// test project, in its own run directory) contributes its JSON report, or,
// when it has none or it cannot be read, its Cobertura report; no run is
// left out, and a run with no readable report is an error naming it. It
// returns the summary note when every threshold is met, an error naming
// every metric when not.
func checkCoverage(runsettings string, th *Thresholds, jsonReports, coberturaReports []string) (string, error) {
	checks, err := measureCoverage(runsettings, th, jsonReports, coberturaReports)
	if err != nil {
		return "", err
	}
	parts := make([]string, len(checks))
	met := true
	for i, c := range checks {
		parts[i] = c.String()
		met = met && c.Met
	}
	if !met {
		return "", fmt.Errorf("coverage below the thresholds of %s (%s): %s", th.origin(runsettings), th.Stat, strings.Join(parts, ", "))
	}
	return fmt.Sprintf("coverage (%s): %s", th.Stat, strings.Join(parts, ", ")), nil
}

// measureCoverage reads the run's reports (see checkCoverage) and compares
// them with every set threshold.
func measureCoverage(runsettings string, th *Thresholds, jsonReports, coberturaReports []string) ([]MetricCheck, error) {
	type run struct{ json, cobertura string }
	runs := map[string]*run{}
	get := func(dir string) *run {
		if runs[dir] == nil {
			runs[dir] = &run{}
		}
		return runs[dir]
	}
	for _, p := range jsonReports {
		get(filepath.Dir(p)).json = p
	}
	for _, p := range coberturaReports {
		get(filepath.Dir(p)).cobertura = p
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("no coverage report to check the thresholds of %s against", th.origin(runsettings))
	}
	cov := newCoverage()
	for _, dir := range slices.Sorted(maps.Keys(runs)) {
		r := runs[dir]
		var problems []string
		read := false
		if r.json != "" {
			if err := cov.addJSON(r.json); err != nil {
				problems = append(problems, err.Error())
			} else {
				read = true
			}
		}
		if !read && r.cobertura != "" {
			if err := cov.addCobertura(r.cobertura); err != nil {
				problems = append(problems, err.Error())
			} else {
				read = true
			}
		}
		if !read {
			return nil, fmt.Errorf("no readable coverage report in run %s: %s", dir, strings.Join(problems, "; "))
		}
	}
	return checkThresholds(cov, th), nil
}
