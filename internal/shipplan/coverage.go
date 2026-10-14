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

// Coverage is the merged line coverage of a set of Cobertura reports.
type Coverage struct {
	// Covered and Valid are the summed lines-covered and lines-valid.
	Covered, Valid int
	// Reports are the report files read, relative to the searched directory.
	Reports []string
}

// Percent is the line coverage, 0-100.
func (c Coverage) Percent() float64 {
	if c.Valid == 0 {
		return 0
	}
	return 100 * float64(c.Covered) / float64(c.Valid)
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
	var c Coverage
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
	reports := fmt.Sprintf("%d report", len(c.Reports))
	if len(c.Reports) > 1 {
		reports += "s"
	}
	if c.Valid == 0 {
		return c, fmt.Errorf("no coverable lines in %s under %s", reports, dir)
	}
	if c.Percent() < threshold {
		return c, fmt.Errorf("line coverage %.2f%% (%d of %d lines, %s) is below the threshold %s%%",
			c.Percent(), c.Covered, c.Valid, reports, strconv.FormatFloat(threshold, 'f', -1, 64))
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
