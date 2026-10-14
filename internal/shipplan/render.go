package shipplan

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// Render lists every step of the plan per stage, for `nerv ship --dry-run`:
// preset commands as their argument lists (quoted for reading only),
// overrides and hooks as [shell] run, checks by what they check. Secrets
// are shown as $<variable>, never their values. The publish stage is
// marked as run at semantic-release's publish step, or not run in a dry
// run; the lifecycle hooks follow.
func (p *Plan) Render() string {
	var b strings.Builder
	channel := "default channel"
	if !p.Context.defaultChannel() {
		channel = "channel " + p.Context.Channel
	}
	fmt.Fprintf(&b, "%s %s (%s", p.Project, p.Context.Version, channel)
	if p.Context.Tag != "" {
		fmt.Fprintf(&b, ", tag %s", p.Context.Tag)
	}
	b.WriteString(")\n")
	for _, st := range p.Stages {
		header := st.Name
		if st.Name == PublishStage {
			if p.Context.DryRun {
				header += " (not run: dry run)"
			} else {
				header += " (runs at semantic-release's publish step)"
			}
		}
		fmt.Fprintf(&b, "  %s\n", header)
		for _, s := range st.Steps {
			fmt.Fprintf(&b, "    %s  %s\n", s.Name, renderStep(s))
		}
	}
	first := true
	for _, key := range shipconfig.LifecycleHooks {
		for _, s := range p.Hooks[key] {
			if first {
				b.WriteString("  hooks at semantic-release steps\n")
				first = false
			}
			fmt.Fprintf(&b, "    %s  %s\n", s.Name, renderStep(s))
		}
	}
	return b.String()
}

// renderStep is one step's command for Render.
func renderStep(s Step) string {
	switch {
	case s.Check != nil:
		return "check " + s.Describe
	case len(s.Exec) > 0:
		parts := make([]string, len(s.Exec))
		for i, a := range s.Exec {
			if a.SecretEnv != "" {
				parts[i] = "$" + a.SecretEnv
			} else {
				parts[i] = quoteArg(a.Value)
			}
		}
		cmd := strings.Join(parts, " ")
		if s.Command.Workdir != "" {
			cmd = "[in " + s.Command.Workdir + "] " + cmd
		}
		return cmd
	default:
		where := s.Command.Shell
		if s.Command.Workdir != "" {
			where += " in " + s.Command.Workdir
		}
		run := strings.ReplaceAll(strings.TrimRight(s.Command.Run, "\r\n"), "\r\n", "\n")
		return "[" + where + "] " + strings.ReplaceAll(run, "\n", "\n        ")
	}
}

// quoteArg quotes an argument holding spaces or quotes, for reading.
func quoteArg(a string) string {
	if a == "" || strings.ContainsAny(a, " \t\n\"'") {
		return strconv.Quote(a)
	}
	return a
}
