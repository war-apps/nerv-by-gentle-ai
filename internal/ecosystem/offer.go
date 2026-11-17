package ecosystem

import (
	"fmt"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-kit-by-gentle-ai/selfupgrade"
)

// OfferOptions are "nerv install"/"nerv upgrade"'s ecosystem-tools flags.
type OfferOptions struct {
	// Tools explicitly names the tools to install/upgrade ("--tools
	// a,b"); an unknown name is a refusal. Non-empty skips the confirm
	// prompt entirely (every named tool is selected outright).
	Tools []string
	// NoTools skips the offer entirely: no prompt, nothing selected,
	// Tools is ignored.
	NoTools bool
}

// ConfirmTool asks whether to act on tool, shown with its current state.
type ConfirmTool func(tool Tool, state State) bool

// Selected resolves which catalogue tools this run installs/upgrades, in
// catalogue order:
//   - opts.NoTools selects none;
//   - opts.Tools, non-empty, selects exactly those (an unknown name is a
//     *refusal.Error), asking confirm nothing;
//   - otherwise, a nil confirm (no terminal) selects none; a non-nil one
//     is asked for every tool, in order, with its detected State from
//     states (the zero State when states is nil or has no entry).
func Selected(tools []Tool, opts OfferOptions, confirm ConfirmTool, states map[string]State) ([]Tool, error) {
	if opts.NoTools {
		return nil, nil
	}
	if len(opts.Tools) > 0 {
		var out []Tool
		for _, name := range opts.Tools {
			t, ok := findByName(tools, name)
			if !ok {
				return nil, &refusal.Error{Err: fmt.Errorf("unknown ecosystem tool %q (available: %s)", name, strings.Join(names(tools), ", "))}
			}
			out = append(out, t)
		}
		return out, nil
	}
	if confirm == nil {
		return nil, nil
	}
	var out []Tool
	for _, t := range tools {
		if confirm(t, states[t.Name]) {
			out = append(out, t)
		}
	}
	return out, nil
}

func findByName(tools []Tool, name string) (Tool, bool) {
	for _, t := range tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

func names(tools []Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name
	}
	return out
}

// RenderState is state as "nerv install"/"nerv upgrade" show it next to a
// tool: "not installed"; "installed, version unknown" (an older tool with
// no "version --json"); or "installed (<version>)", plus ", outdated
// (latest <latest>)" or ", up to date" when latest (a release tag) is
// known.
func RenderState(state State, latest string) string {
	if !state.Installed {
		return "not installed"
	}
	if state.VersionUnknown {
		return "installed, version unknown"
	}
	s := fmt.Sprintf("installed (%s)", state.Version)
	if latest == "" {
		return s
	}
	if selfupgrade.IsNewer(latest, state.Version) {
		return s + fmt.Sprintf(", outdated (latest %s)", latest)
	}
	return s + ", up to date"
}
