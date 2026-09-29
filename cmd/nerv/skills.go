package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-gentle-ai/internal/skills"
)

const skillsUsage = `Usage: nerv skills [flags]

Installs or verifies the Claude Code user-scope skills the NERV plugin
defaults reference: external ones via "npx skills add ... -g", gentle-ai
ones verified only.

Flags:
  --dry-run          Print the exact npx command for each missing skill
                      instead of running it
  --json             Print the computed status as a JSON array
  --only name,...    Restrict processing to these skill names
`

// runSkills ports install-skills.ps1's CLI exactly: the human table (or
// --json array, always an array even for one entry), --only rejecting an
// unknown name with exit 1, --dry-run's "npx skills add ..." lines
// (matching the printed remedy lines for gentle-ai gaps and the human
// summary line), and the failure-count-driven exit code.
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

	manifest, err := skills.LoadManifestFS(opts.PluginFS)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}

	var onlyNames []string
	if *only != "" {
		onlyNames = strings.Split(*only, ",")
	}

	entries, err := skills.FilterOnly(manifest, onlyNames)
	if err != nil {
		fmt.Fprintln(stdout, err.Error())
		return 1
	}
	filtered := &skills.Manifest{Schema: manifest.Schema, Skills: entries}

	skillsDir := filepath.Join(home, ".claude", "skills")
	statuses := skills.Status(filtered, skillsDir)

	if *jsonOut {
		writeJSON(stdout, statuses)
	} else {
		fmt.Fprintf(stdout, "%-32s %-10s %-10s %s\n", "name", "kind", "installed", "action")
		for _, s := range statuses {
			fmt.Fprintf(stdout, "%-32s %-10s %-10v %s\n", s.Name, s.Kind, s.Installed, s.Action)
		}
	}

	plan := skills.InstallPlan(statuses)

	alreadyPresentCount := 0
	for _, s := range statuses {
		if s.Action == skills.ActionNone {
			alreadyPresentCount++
		}
	}

	gentleAiGapCount := len(plan.Remedies)
	if !*jsonOut {
		for _, remedy := range plan.Remedies {
			fmt.Fprintln(stdout, remedy)
		}
	}

	installedCount, failureCount := 0, 0
	if *dryRun {
		if !*jsonOut {
			for _, step := range plan.Installs {
				fmt.Fprintf(stdout, "DryRun: npx %s\n", strings.Join(step.Args, " "))
			}
		}
	} else {
		result := skills.Install(context.Background(), opts.Runner, plan)
		installedCount = result.Installed
		failureCount = result.Failed
		if !*jsonOut {
			for _, name := range result.Failures {
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
