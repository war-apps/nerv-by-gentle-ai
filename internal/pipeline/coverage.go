package pipeline

import (
	"encoding/xml"
	"fmt"
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
// Modules are the reports' packages. The collector's Include/Exclude
// filters were already applied when it wrote the reports, so nothing is
// filtered again here. Several reports (one per test project) are merged
// per line, as coverlet merges results (hits added, the best branch
// coverage of a line kept), so a module present in two reports is not
// counted twice.

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
		FriendlyName string `xml:"friendlyName,attr"`
		Config       struct {
			Threshold     string `xml:"Threshold"`
			ThresholdType string `xml:"ThresholdType"`
			ThresholdStat string `xml:"ThresholdStat"`
		} `xml:"Configuration"`
	} `xml:"DataCollectionRunSettings>DataCollectors>DataCollector"`
}

// parseThresholds reads the thresholds of coverlet's collector in a
// runsettings file: Threshold (comma-separated percentages, in the order of
// ThresholdType, default line,branch,method; a single value applies to every
// type), ThresholdStat (total, average or minimum; default minimum, as
// coverlet). nil when the file declares no threshold.
func parseThresholds(data []byte) (*Thresholds, error) {
	var rs runSettings
	if err := xml.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("runsettings: %w", err)
	}
	for _, c := range rs.Collectors {
		if !strings.EqualFold(strings.TrimSpace(c.FriendlyName), "XPlat code coverage") {
			continue
		}
		raw := strings.TrimSpace(c.Config.Threshold)
		if raw == "" {
			return nil, nil
		}
		types := coverageMetrics
		if tt := strings.TrimSpace(c.Config.ThresholdType); tt != "" {
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
		if s := strings.ToLower(strings.TrimSpace(c.Config.ThresholdStat)); s != "" {
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
	return nil, nil
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
	hits     map[int]int
	branches map[int][2]int // line -> covered, total
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
			mod := cov.modules[pkg.Name]
			if mod == nil {
				mod = &covModule{methods: map[string]*covMethod{}}
				cov.modules[pkg.Name] = mod
				cov.order = append(cov.order, pkg.Name)
			}
			for _, cls := range pkg.Classes {
				for _, m := range cls.Methods {
					key := cls.Name + "\x00" + cls.Filename + "\x00" + m.Name + m.Signature
					meth := mod.methods[key]
					if meth == nil {
						meth = &covMethod{hits: map[int]int{}, branches: map[int][2]int{}}
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

// checkCoverage enforces the runsettings thresholds on the run's reports:
// it returns the summary note when they are met, an error naming every
// metric when not, and ("", nil) when there is no threshold to check.
func checkCoverage(repo, runsettings string, reports []string) (string, error) {
	data, err := os.ReadFile(repoPath(repo, runsettings))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", runsettings, err)
	}
	th, err := parseThresholds(data)
	if err != nil || th == nil {
		return "", err
	}
	if len(reports) == 0 {
		return "", fmt.Errorf("no coverage report to check the thresholds of %s against", runsettings)
	}
	cov, err := readCoverage(reports)
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
