package pipeline

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"maps"
	"os"
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

// readJSONCoverage reads and merges coverlet's JSON reports (module ->
// document -> class -> method), adding line and branch hits.
func readJSONCoverage(paths []string) (*coverage, error) {
	cov := &coverage{modules: map[string]*covModule{}}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var rep map[string]map[string]map[string]map[string]jsonMethod
		if err := json.Unmarshal(data, &rep); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		for _, modName := range slices.Sorted(maps.Keys(rep)) {
			mod := cov.module(modName)
			for doc, classes := range rep[modName] {
				for cls, methods := range classes {
					for name, m := range methods {
						key := doc + "\x00" + cls + "\x00" + name
						meth := mod.methods[key]
						if meth == nil {
							meth = newMethod()
							mod.methods[key] = meth
						}
						for line, hits := range m.Lines {
							n, err := strconv.Atoi(line)
							if err != nil {
								return nil, fmt.Errorf("%s: line %q of %s", p, line, name)
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
	}
	return cov, nil
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

// readCoverage reads and merges coverlet's Cobertura reports (the fallback
// when there is no JSON report).
func readCoverage(paths []string) (*coverage, error) {
	cov := &coverage{modules: map[string]*covModule{}}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var rep coberturaReport
		if err := xml.Unmarshal(data, &rep); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		for _, pkg := range rep.Packages {
			mod := cov.module(pkg.Name)
			for _, cls := range pkg.Classes {
				for _, m := range cls.Methods {
					key := cls.Name + "\x00" + cls.Filename + "\x00" + m.Name + m.Signature
					meth := mod.methods[key]
					if meth == nil {
						meth = newMethod()
						mod.methods[key] = meth
					}
					for _, ln := range m.Lines {
						meth.hits[ln.Number] += ln.Hits
						if !strings.EqualFold(ln.Branch, "true") {
							continue
						}
						match := conditionRe.FindStringSubmatch(ln.Condition)
						if match == nil {
							return nil, fmt.Errorf("%s: line %d: unreadable condition-coverage %q", p, ln.Number, ln.Condition)
						}
						covered, _ := strconv.Atoi(match[1])
						total, _ := strconv.Atoi(match[2])
						prev := meth.branches[ln.Number]
						meth.branches[ln.Number] = [2]int{max(prev[0], covered), max(prev[1], total)}
					}
				}
			}
		}
	}
	return cov, nil
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
			for _, h := range meth.branchHits {
				total++
				if h > 0 {
					covered++
				}
			}
			for _, b := range meth.branches {
				covered += b[0]
				total += b[1]
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
	Met      bool    `json:"met"`
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
		out = append(out, MetricCheck{Type: metric, Actual: percentText(h), Required: *req, Met: !(float64(h)/100 < *req)})
	}
	return out
}

// checkCoverage enforces th on the run's reports: the JSON reports when
// there are any, else the Cobertura ones. It returns the summary note when
// every threshold is met, an error naming every metric when not.
func checkCoverage(runsettings string, th *Thresholds, jsonReports, coberturaReports []string) (string, error) {
	var cov *coverage
	var err error
	switch {
	case len(jsonReports) > 0:
		cov, err = readJSONCoverage(jsonReports)
	case len(coberturaReports) > 0:
		cov, err = readCoverage(coberturaReports)
	default:
		return "", fmt.Errorf("no coverage report to check the thresholds of %s against", runsettings)
	}
	if err != nil {
		return "", err
	}
	checks := checkThresholds(cov, th)
	parts := make([]string, len(checks))
	met := true
	for i, c := range checks {
		parts[i] = c.String()
		met = met && c.Met
	}
	if !met {
		return "", fmt.Errorf("coverage below the thresholds of %s (%s): %s", runsettings, th.Stat, strings.Join(parts, ", "))
	}
	return fmt.Sprintf("coverage (%s): %s", th.Stat, strings.Join(parts, ", ")), nil
}
