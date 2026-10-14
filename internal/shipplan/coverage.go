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
)

// Coverage is the merged line coverage of a set of Cobertura reports,
// checked against a threshold.
type Coverage struct {
	// Covered and Valid are the summed lines-covered and lines-valid.
	Covered, Valid int
	// Reports are the report files read, relative to the searched directory.
	Reports []string
	// Threshold is the minimum line coverage percentage checked.
	Threshold float64
}

// Percent is the line coverage, 0-100.
func (c Coverage) Percent() float64 {
	if c.Valid == 0 {
		return 0
	}
	return 100 * float64(c.Covered) / float64(c.Valid)
}

// below reports whether the coverage is under the threshold, compared
// exactly (covered/valid against threshold/100, without a rounded rate).
func (c Coverage) below() bool {
	return float64(c.Covered)*100 < c.Threshold*float64(c.Valid)
}

// measure is the coverage as text, e.g. "line coverage 79.99% (79996 of
// 100000 lines, 1 report)". The percentage is truncated, never rounded up,
// to two decimals (computed in integers), so a rate under the threshold
// never reads as the threshold.
func (c Coverage) measure() string {
	hundredths := 0
	if c.Valid > 0 {
		hundredths = int(int64(c.Covered) * 10000 / int64(c.Valid))
	}
	return fmt.Sprintf("line coverage %d.%02d%% (%d of %d lines, %s)", hundredths/100, hundredths%100, c.Covered, c.Valid, reportCount(len(c.Reports)))
}

// reportCount is "1 report" or "<n> reports".
func reportCount(n int) string {
	if n == 1 {
		return "1 report"
	}
	return fmt.Sprintf("%d reports", n)
}

func (c Coverage) threshold() string { return strconv.FormatFloat(c.Threshold, 'f', -1, 64) + "%" }

// String is the summary of a coverage that met its threshold.
func (c Coverage) String() string { return c.measure() + ", threshold " + c.threshold() }

// belowError is the failure of a coverage under its threshold.
func (c Coverage) belowError() error {
	return fmt.Errorf("%s is below the threshold %s", c.measure(), c.threshold())
}

// CheckCoverage reads every Cobertura report (a file named
// *.cobertura.xml, such as coverlet's coverage.cobertura.xml) under dir,
// merges them by summing their lines-covered and lines-valid (several test
// projects give several reports; their rates are not averaged), and fails
// when the line coverage is below threshold percent. It also fails when
// there is no report, a report cannot be read, or no line is coverable.
//
// Lines covered by two test projects' reports count twice, once in each:
// coverlet reports per test run, and the counts cannot be merged per line
// from the summary alone.
func CheckCoverage(dir string, threshold float64) (Coverage, error) {
	c := Coverage{Threshold: threshold}
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
		covered, valid, err := readCobertura(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return c, fmt.Errorf("%s: %w", rel, err)
		}
		c.Covered += covered
		c.Valid += valid
	}
	if c.Valid == 0 {
		return c, fmt.Errorf("no coverable lines in %s under %s", reportCount(len(c.Reports)), dir)
	}
	if c.below() {
		return c, c.belowError()
	}
	return c, nil
}

// readCobertura returns the line counts of one Cobertura report's root
// <coverage> element.
func readCobertura(path string) (covered, valid int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	for {
		tok, err := dec.Token()
		if err != nil {
			return 0, 0, fmt.Errorf("not a Cobertura report: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "coverage" {
			return 0, 0, fmt.Errorf("not a Cobertura report (root element <%s>)", start.Name.Local)
		}
		attr := map[string]string{}
		for _, a := range start.Attr {
			attr[a.Name.Local] = a.Value
		}
		covered, err1 := strconv.Atoi(attr["lines-covered"])
		valid, err2 := strconv.Atoi(attr["lines-valid"])
		if err := errors.Join(err1, err2); err != nil || covered < 0 || valid < 0 || covered > valid {
			return 0, 0, errors.New("lines-covered / lines-valid missing or invalid")
		}
		// Read to the end so a truncated file is reported as malformed.
		for {
			if _, err := dec.Token(); err != nil {
				if errors.Is(err, io.EOF) {
					return covered, valid, nil
				}
				return 0, 0, fmt.Errorf("not a Cobertura report: %w", err)
			}
		}
	}
}
