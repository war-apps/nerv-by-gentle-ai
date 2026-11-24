package main

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/ecosystem"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-kit-by-gentle-ai/selfupgrade"
)

// ecosystemToolsFlagsUsage documents --tools/--no-tools, shared by
// install's and upgrade's usage strings. verb is "install" or "upgrade".
func ecosystemToolsFlagsUsage(verb string) string {
	return fmt.Sprintf(`  --tools <names>       Comma-separated NERV-ecosystem tools to %s (such as
                        nerv-task,nerv-release), skipping the confirm prompt
  --no-tools            Skip the NERV-ecosystem tools step entirely
`, verb)
}

// offerEcosystemTools runs the ecosystem-tools step of "nerv install"/
// "nerv upgrade" (S11): detects every catalogue tool's state, then runs
// action ("install": the tool's official script; "upgrade": "<tool>
// upgrade") for the tools toolsFlag names (an unknown name is a
// *refusal.Error), noTools skips entirely, or, on a terminal with neither
// flag, the ones confirmed from a one-by-one [y/N] prompt showing each
// tool's state and, best-effort, its latest release. Without a terminal
// and neither flag, it only prints a hint naming --tools/--no-tools. It
// returns the tools this run installed or upgraded successfully (S6: a
// tool chosen here counts as installed when the plugins it replaces are
// disabled; one whose action failed does not).
func offerEcosystemTools(ctx context.Context, stdout io.Writer, opts options, home, action, toolsFlag string, noTools bool) ([]ecosystem.Tool, error) {
	catalogue := ecosystem.Catalogue()
	if len(catalogue) == 0 {
		return nil, nil
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
		return nil, nil
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
		return nil, err
	}
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	var done []ecosystem.Tool
	for _, t := range selected {
		if runEcosystemAction(ctx, stdout, opts, action, t, states[t.Name].Path, goos) {
			done = append(done, t)
		}
	}
	return done, nil
}

// runEcosystemAction runs action ("install" or "upgrade") for t through
// opts.Runner, reports the result and whether it succeeded; a failed or
// non-zero run is a warning, never a refusal (the tool's own script
// already explains itself on its output).
func runEcosystemAction(ctx context.Context, stdout io.Writer, opts options, action string, t ecosystem.Tool, path, goos string) bool {
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
		return false
	case code != 0:
		fmt.Fprintf(stdout, "Warning: %s %s exited %d\n", action, t.Name, code)
		return false
	}
	return true
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

// orphanFamilies is, per catalogue tool, the extension plugins of the
// nerv marketplace it replaces, which nerv no longer ships (S6): disabled
// by nerv install and nerv upgrade, quietly when the tool is installed on
// this machine or selected (installed or upgraded) in this same run by
// offerEcosystemTools, with a warning naming the tool otherwise.
func orphanFamilies(ctx context.Context, opts options, home string, selected []ecosystem.Tool) []install.OrphanFamily {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	var families []install.OrphanFamily
	for _, t := range ecosystem.Catalogue() {
		installed := slices.ContainsFunc(selected, func(s ecosystem.Tool) bool { return s.Name == t.Name }) ||
			ecosystem.Detect(ctx, t, opts.Runner, opts.LookPath, goos, home, opts.Getenv).Installed
		families = append(families, install.OrphanFamily{Tool: t.Name, Commands: t.Commands, Plugins: t.Replaces, ToolInstalled: installed})
	}
	return families
}

// disableOrphans is nerv upgrade's orphan step (S6), after its own
// ecosystem-tools step: install.DisableOrphans over orphanFamilies. A
// failure is a warning; it never changes the exit code.
func disableOrphans(ctx context.Context, stdout io.Writer, opts options, home string, selected []ecosystem.Tool) {
	deps := install.Deps{Home: home, SettingsPath: opts.Settings, Runner: opts.Runner, Now: opts.Now, Stdout: stdout}
	if err := install.DisableOrphans(ctx, deps, orphanFamilies(ctx, opts, home, selected)); err != nil {
		fmt.Fprintf(stdout, "Warning: disabling the extension plugins nerv no longer ships: %v\n", err)
	}
}

// offerToolMigration offers, after the ecosystem-tools step of "nerv
// install"/"nerv upgrade" (S3), the user-scope nerv migrate when a
// catalogue tool is installed (including one this run just installed)
// and its settings are still in the user config.yaml: the steps nerv
// migrate --user would take, as its dry run shows them, then a [y/N]
// prompt on a terminal; without one, only a hint naming the command.
// Nothing is written without a "y". It covers the user configuration
// only: install and upgrade work on no repository (nerv migrate and nerv
// doctor cover the repository's own move). It never changes the exit
// code: a plan that cannot be computed or applied is a warning.
func offerToolMigration(ctx context.Context, stdout io.Writer, opts options, home string) {
	o := migrate.Options{Home: home, User: true, Now: opts.Now()}
	setToolFileTargets(ctx, opts, home, &o)
	if o.TaskYAMLHome == "" && o.ReleaseYAMLHome == "" {
		return
	}
	plan, err := migrate.Compute(o)
	if err != nil {
		fmt.Fprintf(stdout, "\nWarning: nerv migrate --user cannot run: %v\n", err)
		return
	}
	moves := pendingToolFileMoves(plan, o)
	if len(moves) == 0 {
		return
	}
	var tools []string
	for _, m := range moves {
		tools = append(tools, m.Tool)
	}
	subject := strings.Join(tools, " and ") + " reads its settings from its own file, but your user config.yaml still has them"
	if len(tools) > 1 {
		subject = strings.Join(tools, " and ") + " read their settings from their own files, but your user config.yaml still has them"
	}
	if opts.IsTerminal == nil || !opts.IsTerminal() || opts.Stdin == nil {
		fmt.Fprintf(stdout, "\n%s: run `nerv migrate --user --dry-run` to see the steps, then `nerv migrate --user`.\n", subject)
		return
	}
	fmt.Fprintln(stdout, "\n=== Move settings to the NERV-ecosystem tools ===")
	fmt.Fprintf(stdout, "%s. nerv migrate --user would take these steps:\n", subject)
	fmt.Fprint(stdout, migrate.Render(plan))
	fmt.Fprint(stdout, "Run nerv migrate --user now? [y/N]: ")
	line, sawNewline := readAnswer(opts.Stdin)
	if !sawNewline {
		fmt.Fprintln(stdout)
	}
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		fmt.Fprintln(stdout, "Not migrated: run `nerv migrate --user` when you are ready.")
		return
	}
	backups, err := migrate.Apply(plan, o.Now)
	if len(backups) > 0 {
		fmt.Fprintln(stdout, "Backups:")
		for _, b := range backups {
			fmt.Fprintf(stdout, "  - %s\n", b)
		}
	}
	if err != nil {
		fmt.Fprintf(stdout, "Warning: nerv migrate --user: %v\n", err)
		return
	}
	fmt.Fprintf(stdout, "nerv migrate: %d change(s) written\n", len(plan.Changes))
}
