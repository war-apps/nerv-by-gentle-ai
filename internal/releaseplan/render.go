package releaseplan

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
)

// Render lists every step of the plan per stage, for `nerv release`: built-in
// commands as their argument lists (quoted for reading only), stage
// commands and hooks as [shell] run, in-process steps by what they do. The
// lifecycle hooks follow.
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
	if p.Provider != "" {
		fmt.Fprintf(&b, ", build %s", p.Provider)
	}
	b.WriteString(")\n")
	for _, st := range p.Stages {
		fmt.Fprintf(&b, "  %s\n", st.Name)
		for _, s := range st.Steps {
			fmt.Fprintf(&b, "    %s  %s\n", s.Name, renderStep(s))
		}
	}
	first := true
	for _, key := range releaseconfig.LifecycleHooks {
		for _, s := range p.Hooks[key] {
			if first {
				b.WriteString("  hooks (prepare: after the pipeline, before the artifact is written; fail: when a stage or hook fails)\n")
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
	case s.Func != nil:
		return s.Describe
	case len(s.Exec) > 0:
		parts := make([]string, len(s.Exec))
		for i, a := range s.Exec {
			parts[i] = quoteArg(a)
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
