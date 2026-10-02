package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/skills"
)

const skillsUsage = `Usage: nerv skills [flags]

Installs or verifies the Claude Code user-scope skills the NERV plugin
defaults reference: external ones via "npx skills add ... -g", gentle-ai
ones via "gentle-ai sync --agents claude-code --skills ..." (one run for
all the missing ones, after a confirmation in a terminal; without one the
sync never runs and the remedies are printed).

Flags:
  --dry-run          Print the exact npx and gentle-ai commands for the
                      missing skills instead of running them
  --json             Print the computed status as a JSON array
  --only name,...    Restrict processing to these skill names
`

// runSkills is "nerv skills"'s CLI: the human table (or --json array,
// always an array even for one entry), --only rejecting an unknown name
// with exit 1, --dry-run's "npx skills add ..." lines alongside the
// printed remedy lines for the gentle-ai gaps left after "gentle-ai sync"
// (whose own line is printed when it runs or, with --dry-run, would run),
// the human summary line, and the failure-count-driven exit code.
func runSkills(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("skills", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	dryRun := fs.Bool("dry-run", false, "")
	jsonOut := fs.Bool("json", false, "")
	only := fs.String("only", "", "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprint(stderr, skillsUsage)
		return 2
	}

	home, err := configure.ResolveHome(opts.Home)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}

	var onlyNames []string
	if *only != "" {
		onlyNames = strings.Split(*only, ",")
	}

	skillsDir := paths.Resolve(home).SkillsDir
	report, err := skills.Run(context.Background(), opts.Runner, opts.PluginFS, skillsDir, skills.Options{
		Only:        onlyNames,
		DryRun:      *dryRun,
		ConfirmSync: syncConfirmer(opts, stdout),
	})
	if err != nil {
		var manifestErr *skills.ErrManifest
		if errors.As(err, &manifestErr) {
			fmt.Fprintf(stdout, "nerv: %v\n", manifestErr.Err)
			return 2
		}
		fmt.Fprintln(stdout, err.Error())
		return 1
	}

	if *jsonOut {
		writeJSON(stdout, report.Statuses)
	} else {
		fmt.Fprintf(stdout, "%-32s %-10s %-10s %s\n", "name", "kind", "installed", "action")
		for _, s := range report.Statuses {
			fmt.Fprintf(stdout, "%-32s %-10s %-10v %s\n", s.Name, s.Kind, s.Installed, s.Action)
		}
	}

	alreadyPresentCount := 0
	for _, s := range report.Statuses {
		if s.Action == skills.ActionNone {
			alreadyPresentCount++
		}
	}

	gentleAiGapCount := len(report.Plan.Remedies)
	if !*jsonOut {
		skills.RenderSync(stdout, report)
		for _, remedy := range report.Plan.Remedies {
			fmt.Fprintln(stdout, remedy)
		}
	}

	installedCount, failureCount := 0, 0
	if *dryRun {
		if !*jsonOut {
			for _, step := range report.Plan.Installs {
				fmt.Fprintf(stdout, "DryRun: npx %s\n", strings.Join(step.Args, " "))
			}
			if report.Plan.Sync != nil {
				fmt.Fprintf(stdout, "DryRun: gentle-ai %s\n", strings.Join(report.Plan.Sync.Args, " "))
			}
		}
	} else {
		installedCount = report.Result.Installed
		failureCount = report.Result.Failed
		if !*jsonOut {
			for _, name := range report.Result.Failures {
				fmt.Fprintf(stdout, "FAILED to install skill '%s'.\n", name)
			}
		}
	}

	if !*jsonOut {
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "skills: %d installed, %d already present, %d gentle-ai gaps, %d failures\n",
			installedCount, alreadyPresentCount, gentleAiGapCount, failureCount)
	}

	if failureCount > 0 {
		return 1
	}
	return 0
}
