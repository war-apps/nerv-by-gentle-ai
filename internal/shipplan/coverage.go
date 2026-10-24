package shipplan

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// Coverage is the merged coverage of a set of Cobertura reports, checked
// against line, branch and method floors.
type Coverage struct {
	// Covered and Valid are the summed lines-covered and lines-valid.
	Covered, Valid int
	// BranchesCovered and BranchesValid are the summed branches-covered and
	// branches-valid, read only when a branch floor is checked.
	BranchesCovered, BranchesValid int
	// MethodsCovered and MethodsValid count the reports' <method> elements,
	// a method being covered when one of its lines has a hit (coverlet's
	// method coverage).
	MethodsCovered, MethodsValid int
	// Reports are the report files read, relative to the searched directory.
	Reports []string
	// Threshold is the minimum line coverage percentage checked (0: not
	// checked).
	Threshold float64
	// BranchThreshold and MethodThreshold are the branch and method floors
	// (0: not checked).
	BranchThreshold, MethodThreshold float64
}

// Percent is the line coverage, 0-100.
func (c Coverage) Percent() float64 {
	if c.Valid == 0 {
		return 0
	}
	return 100 * float64(c.Covered) / float64(c.Valid)
}

// metric is one checked floor: its counts, unit and threshold.
type metric struct {
	name, unit      string
	covered, valid  int
	threshold       float64
	emptyIsComplete bool
}

// metrics are the floors checked, in coverlet's order (line, branch,
// method). A branch or method metric with nothing to cover is 100%, as
// coverlet counts it.
func (c Coverage) metrics() []metric {
	var out []metric
	for _, m := range []metric{
		{"line", "lines", c.Covered, c.Valid, c.Threshold, false},
		{"branch", "branches", c.BranchesCovered, c.BranchesValid, c.BranchThreshold, true},
		{"method", "methods", c.MethodsCovered, c.MethodsValid, c.MethodThreshold, true},
	} {
		if m.threshold > 0 {
			out = append(out, m)
		}
	}
	return out
}

// below reports whether the metric is under its threshold, compared
// exactly (covered/valid against threshold/100, without a rounded rate).
func (m metric) below() bool {
	if m.valid == 0 && m.emptyIsComplete {
		return false
	}
	return float64(m.covered)*100 < m.threshold*float64(m.valid)
}

// measure is the metric as text, e.g. "line coverage 79.99% (79996 of
// 100000 lines, 1 report)". The percentage is truncated, never rounded up,
// to two decimals (computed in integers), so a rate under the threshold
// never reads as the threshold.
func (m metric) measure(reports int) string {
	hundredths := 0
	switch {
	case m.valid > 0:
		hundredths = int(int64(m.covered) * 10000 / int64(m.valid))
	case m.emptyIsComplete:
		hundredths = 10000
	}
	return fmt.Sprintf("%s coverage %d.%02d%% (%d of %d %s, %s)", m.name, hundredths/100, hundredths%100, m.covered, m.valid, m.unit, reportCount(reports))
}

func (m metric) thresholdText() string { return strconv.FormatFloat(m.threshold, 'f', -1, 64) + "%" }

// reportCount is "1 report" or "<n> reports".
func reportCount(n int) string {
	if n == 1 {
		return "1 report"
	}
	return fmt.Sprintf("%d reports", n)
}

// String is the summary of a coverage that met its floors: each floor's
// measure and threshold, joined by "; ".
func (c Coverage) String() string {
	parts := make([]string, 0, 3)
	for _, m := range c.metrics() {
		parts = append(parts, m.measure(len(c.Reports))+", threshold "+m.thresholdText())
	}
	return strings.Join(parts, "; ")
}

// belowError is the failure of a coverage under a floor: every floor, the
// ones missed as "<measure> is below the threshold <t>". With a line floor
// alone it is the line message coverage_threshold always gave.
func (c Coverage) belowError() error {
	parts := make([]string, 0, 3)
	for _, m := range c.metrics() {
		if m.below() {
			parts = append(parts, m.measure(len(c.Reports))+" is below the threshold "+m.thresholdText())
		} else {
			parts = append(parts, m.measure(len(c.Reports))+", threshold "+m.thresholdText())
		}
	}
	return errors.New(strings.Join(parts, "; "))
}

// CheckCoverage is CheckCoverageFloors with a line floor alone.
func CheckCoverage(dir string, threshold float64) (Coverage, error) {
	return CheckCoverageFloors(dir, shipconfig.CoverageThreshold{Line: threshold})
}

// CheckCoverageFloors reads every Cobertura report (a file named
// *.cobertura.xml, such as coverlet's coverage.cobertura.xml) under dir,
// merges them by summing their counts (several test projects give several
// reports; their rates are not averaged), and fails when a floor of
// floors (line, branch, method; 0 is not checked) is missed. It also fails
// when there is no report, a report cannot be read, a branch floor meets a
// report without branch counts, or a line floor finds no coverable line.
//
// Lines, branches and methods covered by two test projects' reports count
// twice, once in each: coverlet reports per test run, and the counts cannot
// be merged per line from the summary alone.
func CheckCoverageFloors(dir string, floors shipconfig.CoverageThreshold) (Coverage, error) {
	c := Coverage{Threshold: floors.Line, BranchThreshold: floors.Branch, MethodThreshold: floors.Method}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".cobertura.xml") {
			rel, _ := filepath.Rel(dir, p)
			c.Reports = append(c.Reports, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return c, fmt.Errorf("reading coverage reports: %w", err)
	}
	if len(c.Reports) == 0 {
		return c, fmt.Errorf("no Cobertura report (*.cobertura.xml) under %s: the tests did not collect coverage", dir)
	}
	slices.Sort(c.Reports)
	for _, rel := range c.Reports {
		r, err := readCobertura(filepath.Join(dir, filepath.FromSlash(rel)), floors.Branch > 0)
		if err != nil {
			return c, fmt.Errorf("%s: %w", rel, err)
		}
		c.Covered += r.linesCovered
		c.Valid += r.linesValid
		c.BranchesCovered += r.branchesCovered
		c.BranchesValid += r.branchesValid
		c.MethodsCovered += r.methodsCovered
		c.MethodsValid += r.methodsValid
	}
	if c.Valid == 0 && floors.Line > 0 {
		return c, fmt.Errorf("no coverable lines in %s under %s", reportCount(len(c.Reports)), dir)
	}
	for _, m := range c.metrics() {
		if m.below() {
			return c, c.belowError()
		}
	}
	return c, nil
}

// coberturaCounts are one report's counts.
type coberturaCounts struct {
	linesCovered, linesValid       int
	branchesCovered, branchesValid int
	methodsCovered, methodsValid   int
}

// readCobertura returns the counts of one Cobertura report: the line (and,
// with branches, the branch) counts of its root <coverage> element, and its
// methods, each covered when one of its <line> elements has hits > 0.
func readCobertura(path string, branches bool) (coberturaCounts, error) {
	var r coberturaCounts
	f, err := os.Open(path)
	if err != nil {
		return r, err
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	for {
		tok, err := dec.Token()
		if err != nil {
			return r, fmt.Errorf("not a Cobertura report: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "coverage" {
			return r, fmt.Errorf("not a Cobertura report (root element <%s>)", start.Name.Local)
		}
		attr := map[string]string{}
		for _, a := range start.Attr {
			attr[a.Name.Local] = a.Value
		}
		var ok1, ok2 bool
		if r.linesCovered, r.linesValid, ok1 = counts(attr, "lines"); !ok1 {
			return r, errors.New("lines-covered / lines-valid missing or invalid")
		}
		if branches {
			if r.branchesCovered, r.branchesValid, ok2 = counts(attr, "branches"); !ok2 {
				return r, errors.New("branches-covered / branches-valid missing or invalid")
			}
		}
		// Read to the end, counting methods, so a truncated file is
		// reported as malformed.
		inMethod, methodHit := false, false
		for {
			tok, err := dec.Token()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return r, nil
				}
				return r, fmt.Errorf("not a Cobertura report: %w", err)
			}
			switch t := tok.(type) {
			case xml.StartElement:
				switch {
				case t.Name.Local == "method":
					inMethod, methodHit = true, false
				case t.Name.Local == "line" && inMethod && !methodHit:
					for _, a := range t.Attr {
						if a.Name.Local == "hits" {
							n, err := strconv.ParseInt(a.Value, 10, 64)
							methodHit = err == nil && n > 0
						}
					}
				}
			case xml.EndElement:
				if t.Name.Local == "method" && inMethod {
					r.methodsValid++
					if methodHit {
						r.methodsCovered++
					}
					inMethod = false
				}
			}
		}
	}
}

// counts reads the <kind>-covered and <kind>-valid attributes: present,
// non-negative, and covered at most valid.
func counts(attr map[string]string, kind string) (covered, valid int, ok bool) {
	covered, err1 := strconv.Atoi(attr[kind+"-covered"])
	valid, err2 := strconv.Atoi(attr[kind+"-valid"])
	if errors.Join(err1, err2) != nil || covered < 0 || valid < 0 || covered > valid {
		return 0, 0, false
	}
	return covered, valid, true
}
