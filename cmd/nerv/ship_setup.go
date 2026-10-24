package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/war-apps/nerv-by-gentle-ai/internal/atomicfile"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

const shipSetupUsage = `Usage: nerv ship setup --import [--dry-run] [--cwd <dir>]

Imports, once, the repository's semantic-release configuration (package.json
"release", .releaserc, .releaserc.json, .releaserc.yaml or .yml) into the
ship: section of .ai-workflow.yml: its branches, release rules, tag format,
repository URL and changelog become the native version engine's settings.
The section's other keys (the projects and their builds) and the rest of
the file are kept; an old release: section is renamed to ship:. Each part
that cannot be translated is printed as a warning (the npm and exec
plugins, presetConfig.types, extends, maintenance branches; git and github
belong to the deploy: section). A JavaScript configuration is never
imported: it is reported, to migrate by hand. After the import the
semantic-release configuration is no longer used.

Flags:
  --import       Import the semantic-release configuration (required)
  --dry-run      Print the new ship: section; write nothing
  --cwd <dir>    A directory inside the repository (default ".")

Exit codes: 0 imported (or a dry run), 1 refused (no semantic-release
configuration, a JavaScript one, a result that does not load), 2 usage or
environment error.
`

// runShipSetup is `nerv ship setup`.
func runShipSetup(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("ship setup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cwd := fs.String("cwd", ".", "")
	doImport := fs.Bool("import", false, "")
	dryRun := fs.Bool("dry-run", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, shipSetupUsage)
			return 0
		}
		return shipSetupUsageError(stderr, err.Error())
	}
	if fs.NArg() > 0 {
		return shipSetupUsageError(stderr, fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}
	if !*doImport {
		return shipSetupUsageError(stderr, "--import is required (it is the only setup mode; the build settings are written by /ship:setup)")
	}
	err := shipSetupImport(context.Background(), *cwd, *dryRun, stdout, stderr, opts)
	if err == nil {
		return 0
	}
	fmt.Fprintf(stderr, "nerv ship setup: %v\n", err)
	var invalid *shipconfig.ValidationError
	if refusal.Is(err) || errors.As(err, &invalid) {
		return 1
	}
	return exitCodeFor(err)
}

func shipSetupUsageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "nerv ship setup: %s\n\n%s", problem, shipSetupUsage)
	return 2
}

func shipSetupImport(ctx context.Context, cwd string, dryRun bool, stdout, stderr io.Writer, opts options) error {
	top, err := gitTopLevel(ctx, opts.Runner, cwd)
	if err != nil {
		return err
	}
	imported, err := versioning.Import(top)
	var manual *versioning.ManualMigrationError
	switch {
	case errors.As(err, &manual):
		return &refusal.Error{Err: fmt.Errorf("%w; write the ship: section's branches and release_rules from it (see /ship:setup)", err)}
	case errors.Is(err, versioning.ErrNoSemanticReleaseConfig):
		return &refusal.Error{Err: fmt.Errorf("%w in %s (package.json \"release\", .releaserc, .releaserc.json, .releaserc.yaml or .yml): nothing to import", err, top)}
	case err != nil:
		return &refusal.Error{Err: err}
	}
	path := paths.ProjectConfig(top)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	text, notes, err := shipconfig.ImportSection(data, imported.Config)
	if err != nil {
		return err
	}
	for _, w := range imported.Warnings {
		fmt.Fprintf(stderr, "nerv ship setup: warning: %s\n", w)
	}
	for _, n := range notes {
		fmt.Fprintf(stderr, "nerv ship setup: %s\n", n)
	}
	if dryRun {
		block, _ := config.Parse(text).Block(shipconfig.SectionKey)
		fmt.Fprintf(stdout, "%s\n", block)
		fmt.Fprintf(stderr, "nerv ship setup: dry run: %s was not written (imported from %s)\n", path, imported.Source)
		return nil
	}
	if err := atomicfile.Write(path, []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "nerv ship setup: imported %s into the ship: section of %s (%d warning(s))\n", imported.Source, path, len(imported.Warnings))
	return nil
}
