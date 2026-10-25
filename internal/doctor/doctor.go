// Package doctor holds the result model and the report of "nerv doctor":
// each check reports a status, a one-line detail and, when something can
// be done about it, the step that does it.
package doctor

import (
	"fmt"
	"strings"
)

// Status is the outcome of one check.
type Status string

const (
	// OK means the check found nothing to do.
	OK Status = "ok"
	// Info means an optional part is not set up; its step is a suggestion.
	Info Status = "info"
	// Warn means NERV works, but something should be changed.
	Warn Status = "warn"
	// Fail means NERV cannot work until it is fixed.
	Fail Status = "fail"
)

// Result is one check's outcome.
type Result struct {
	// Section is the part of the report the check belongs to, such as
	// "This machine".
	Section string `json:"section"`
	// Name is the check's short name, such as "gentle-ai" or "migration".
	Name   string `json:"name"`
	Status Status `json:"status"`
	// Detail says what the check found.
	Detail string `json:"detail"`
	// Step is what to do about it, empty when there is nothing to do.
	Step string `json:"step,omitempty"`
}

// Report is the outcome of every check, in order.
type Report struct {
	Results []Result `json:"results"`
	section string
}

// Section starts a part of the report: the results added next belong to it.
func (r *Report) Section(title string) { r.section = title }

// Add appends a result to the current section.
func (r *Report) Add(name string, status Status, detail, step string) {
	r.Results = append(r.Results, Result{Section: r.section, Name: name, Status: status, Detail: detail, Step: step})
}

// Count is the number of results with status s.
func (r Report) Count(s Status) int {
	n := 0
	for _, res := range r.Results {
		if res.Status == s {
			n++
		}
	}
	return n
}

// Health is "healthy" with no warning or failure, "needs attention" with
// warnings only, and "not ready" with any failure.
func (r Report) Health() string {
	switch {
	case r.Count(Fail) > 0:
		return "not ready"
	case r.Count(Warn) > 0:
		return "needs attention"
	default:
		return "healthy"
	}
}

var marks = map[Status]string{OK: "[ok]", Info: "[--]", Warn: "[!!]", Fail: "[xx]"}

// Render is the report for a person: one line per check under its
// section's title, the summary, then the steps to take (failures first,
// then warnings) and the optional ones. The same step is listed once.
func Render(r Report) string {
	var b strings.Builder
	width := 0
	for _, res := range r.Results {
		width = max(width, len(res.Name))
	}
	for i, res := range r.Results {
		if i == 0 || res.Section != r.Results[i-1].Section {
			if i > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "%s:\n", res.Section)
		}
		fmt.Fprintf(&b, "  %s  %-*s  %s\n", marks[res.Status], width, res.Name, res.Detail)
	}
	fmt.Fprintf(&b, "\nSummary: %d ok, %d warning(s), %d failed: %s\n", r.Count(OK)+r.Count(Info), r.Count(Warn), r.Count(Fail), r.Health())
	if steps := r.steps(Fail, Warn); len(steps) > 0 {
		b.WriteString("\nNext steps:\n")
		for i, s := range steps {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, s)
		}
	}
	if steps := r.steps(Info); len(steps) > 0 {
		b.WriteString("\nOptional:\n")
		for _, s := range steps {
			fmt.Fprintf(&b, "  - %s\n", s)
		}
	}
	return b.String()
}

// steps are the distinct steps of the results with the given statuses, in
// the order of the statuses, then of the results.
func (r Report) steps(statuses ...Status) []string {
	var steps []string
	seen := map[string]bool{}
	for _, s := range statuses {
		for _, res := range r.Results {
			if res.Status == s && res.Step != "" && !seen[res.Step] {
				seen[res.Step] = true
				steps = append(steps, res.Step)
			}
		}
	}
	return steps
}
