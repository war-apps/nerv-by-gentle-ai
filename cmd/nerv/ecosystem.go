package main

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/ecosystem"
	"github.com/war-apps/nerv-kit-by-gentle-ai/selfupgrade"
)

// ecosystemToolsFlagsUsage documents --tools/--no-tools, shared by
// install's and upgrade's usage strings. verb is "install" or "upgrade".
func ecosystemToolsFlagsUsage(verb string) string {
	return fmt.Sprintf(`  --tools <names>       Comma-separated NERV-ecosystem tools to %s (such as
                        nerv-task), skipping the confirm prompt
  --no-tools            Skip the NERV-ecosystem tools step entirely
`, verb)
}

// offerEcosystemTools runs the ecosystem-tools step of "nerv install"/
// "nerv upgrade" (S11), after nerv's own work: detects every catalogue
// tool's state, then runs action ("install": the tool's official script;
// "upgrade": "<tool> upgrade") for the tools toolsFlag names (an unknown
// name is a *refusal.Error), noTools skips entirely, or, on a terminal
// with neither flag, the ones confirmed from a one-by-one [y/N] prompt
// showing each tool's state and, best-effort, its latest release. Without
// a terminal and neither flag, it only prints a hint naming
// --tools/--no-tools.
func offerEcosystemTools(ctx context.Context, stdout io.Writer, opts options, home, action, toolsFlag string, noTools bool) error {
	catalogue := ecosystem.Catalogue()
	if len(catalogue) == 0 {
		return nil
	}
	offerOpts := ecosystem.OfferOptions{Tools: splitList(toolsFlag), NoTools: noTools}
	interactive := !offerOpts.NoTools && len(offerOpts.Tools) == 0

	if interactive && (opts.IsTerminal == nil || !opts.IsTerminal() || opts.Stdin == nil) {
		names := make([]string, len(catalogue))
		for i, t := range catalogue {
			names[i] = t.Name
		}
		fmt.Fprintf(stdout, "\nRun `nerv %s --tools <name,...>` to %s a NERV-ecosystem tool (%s); --no-tools skips this.\n",
			action, action, strings.Join(names, ", "))
		return nil
	}

	states := map[string]ecosystem.State{}
	var confirm ecosystem.ConfirmTool
	if interactive {
		goos := opts.GOOS
		if goos == "" {
			goos = runtime.GOOS
		}
		verb := "Install"
		if action == "upgrade" {
			verb = "Upgrade"
		}
		fmt.Fprintln(stdout, "\n=== NERV-ecosystem tools ===")
		for _, t := range catalogue {
			state := ecosystem.Detect(ctx, t, opts.Runner, opts.LookPath, goos, home, opts.Getenv)
			states[t.Name] = state
			latest, err := ecosystemLatest(ctx, opts, t)
			if err != nil {
				latest = ""
			}
			fmt.Fprintf(stdout, "%s: %s — %s\n", t.Name, ecosystem.RenderState(state, latest), t.Description)
		}
		confirm = func(t ecosystem.Tool, _ ecosystem.State) bool {
			fmt.Fprintf(stdout, "%s %s? [y/N]: ", verb, t.Name)
			line, sawNewline := readAnswer(opts.Stdin)
			if !sawNewline {
				fmt.Fprintln(stdout)
			}
			answer := strings.ToLower(strings.TrimSpace(line))
			return answer == "y" || answer == "yes"
		}
	}

	selected, err := ecosystem.Selected(catalogue, offerOpts, confirm, states)
	if err != nil {
		return err
	}
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	for _, t := range selected {
		runEcosystemAction(ctx, stdout, opts, action, t, states[t.Name].Path, goos)
	}
	return nil
}

// runEcosystemAction runs action ("install" or "upgrade") for t through
// opts.Runner and reports the result; a failed or non-zero run is a
// warning, never a refusal (the tool's own script already explains
// itself on its output).
func runEcosystemAction(ctx context.Context, stdout io.Writer, opts options, action string, t ecosystem.Tool, path, goos string) {
	fmt.Fprintf(stdout, "\n=== %s %s ===\n", strings.ToUpper(action[:1])+action[1:], t.Name)
	var out, errOut string
	var code int
	var err error
	if action == "upgrade" {
		out, errOut, code, err = ecosystem.Upgrade(ctx, opts.Runner, t, path)
	} else {
		out, errOut, code, err = ecosystem.Install(ctx, opts.Runner, t, goos)
	}
	if out != "" {
		fmt.Fprint(stdout, out)
	}
	if errOut != "" {
		fmt.Fprint(stdout, errOut)
	}
	switch {
	case err != nil:
		fmt.Fprintf(stdout, "Warning: %s %s: %v\n", action, t.Name, err)
	case code != 0:
		fmt.Fprintf(stdout, "Warning: %s %s exited %d\n", action, t.Name, code)
	}
}

// ecosystemLatest resolves t's latest stable release, through
// opts.HTTPClient and opts.EcosystemAPIBase[t.Name] (tests only).
func ecosystemLatest(ctx context.Context, opts options, t ecosystem.Tool) (string, error) {
	client := selfupgrade.Client{HTTP: opts.HTTPClient}
	if opts.EcosystemAPIBase != nil {
		client.APIBase = opts.EcosystemAPIBase[t.Name]
	}
	return ecosystem.Latest(ctx, t, client)
}

// suppressedExtensions is the nerv extension names this run must never
// offer, select or re-enable (U2/S11): every catalogue tool's Replaces,
// for a tool already installed on this machine, or named by toolsFlag
// (--tools) in this same run — not noTools, which selects nothing.
func suppressedExtensions(ctx context.Context, opts options, home, toolsFlag string, noTools bool) []string {
	catalogue := ecosystem.Catalogue()
	installedOrSelected := map[string]bool{}
	if !noTools {
		goos := opts.GOOS
		if goos == "" {
			goos = runtime.GOOS
		}
		for _, t := range catalogue {
			if ecosystem.Detect(ctx, t, opts.Runner, opts.LookPath, goos, home, opts.Getenv).Installed {
				installedOrSelected[t.Name] = true
			}
		}
		for _, name := range splitList(toolsFlag) {
			installedOrSelected[name] = true
		}
	}
	return ecosystem.ReplacedExtensions(catalogue, installedOrSelected)
}
