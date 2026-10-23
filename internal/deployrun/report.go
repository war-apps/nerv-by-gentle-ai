package deployrun

import (
	"fmt"
	"io"
)

// The statuses of a step.
const (
	// StatusDone: the step ran and succeeded.
	StatusDone = "done"
	// StatusSkipped: nothing to do, it was already done (or the provider
	// skipped it); Detail says why.
	StatusSkipped = "skipped"
	// StatusFailed: the step ran and failed; the deploy stopped there.
	StatusFailed = "failed"
	// StatusNotRun: an earlier step failed or the deploy was refused.
	StatusNotRun = "not-run"
	// StatusDryRun: a dry run stopped before it.
	StatusDryRun = "dry-run"
)

// Step is one step of a deploy and its outcome.
type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Report is what a deploy did, step by step, in order.
type Report struct {
	Project string `json:"project"`
	Version string `json:"version"`
	Tag     string `json:"tag"`
	// Channel is empty for the default channel.
	Channel string `json:"channel"`
	Branch  string `json:"branch"`
	DryRun  bool   `json:"dry_run"`
	// Commit is the release commit (once known).
	Commit string `json:"commit,omitempty"`
	Steps  []Step `json:"steps"`
}

// Step returns the step named name.
func (r *Report) Step(name string) *Step {
	for i := range r.Steps {
		if r.Steps[i].Name == name {
			return &r.Steps[i]
		}
	}
	return nil
}

// Write prints one line per step.
func (r Report) Write(w io.Writer) {
	for _, s := range r.Steps {
		if s.Detail != "" {
			fmt.Fprintf(w, "  %-22s %-8s %s\n", s.Name, s.Status, s.Detail)
		} else {
			fmt.Fprintf(w, "  %-22s %s\n", s.Name, s.Status)
		}
	}
}

// tracker updates the report's steps as the deploy goes.
type tracker struct {
	report *Report
	out    io.Writer
}

func (t tracker) set(name, status, format string, args ...any) {
	s := t.report.Step(name)
	if s == nil {
		return
	}
	s.Status, s.Detail = status, fmt.Sprintf(format, args...)
	if t.out != nil {
		if s.Detail != "" {
			fmt.Fprintf(t.out, "nerv deploy: %s: %s: %s\n", name, status, s.Detail)
		} else {
			fmt.Fprintf(t.out, "nerv deploy: %s: %s\n", name, status)
		}
	}
}

// rest marks every step still not run with status (dry-run after a dry
// run, not-run after a failure stays as initialised).
func (t tracker) rest(status, detail string) {
	for i := range t.report.Steps {
		if t.report.Steps[i].Status == StatusNotRun {
			t.report.Steps[i].Status, t.report.Steps[i].Detail = status, detail
		}
	}
}
