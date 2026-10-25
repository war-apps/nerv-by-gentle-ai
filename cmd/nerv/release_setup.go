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
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

const releaseSetupUsage = `Usage: nerv release setup --import [--force] [--dry-run] [--cwd <dir>]

Imports, once, the repository's semantic-release configuration (package.json
"release", .releaserc, .releaserc.json, .releaserc.yaml or .yml) into the
release: section of .ai-workflow.yml: its branches, release rules, tag format,
repository URL and changelog become the native version engine's settings.
The section's other keys (the projects and their builds) and the rest of
the file are kept. A 4.2.0 release: section (semantic_release: or the
presets) is refused: nerv migrate converts it. An engine key
the release: section already sets to another value is never replaced
silently: the import refuses, listing each one with its current and
imported values, unless --force is given, which replaces them and prints
each replaced value. Each part
that cannot be translated is printed as a warning (the npm and exec
plugins, presetConfig.types, extends, maintenance branches; git and github
belong to the deploy: section). A JavaScript configuration is never
imported: it is reported, to migrate by hand. After the import the
semantic-release configuration is no longer used.

Flags:
  --import       Import the semantic-release configuration (required)
  --force        Replace engine keys the release: section already sets
  --dry-run      Print the new release: section; write nothing
  --cwd <dir>    A directory inside the repository (default ".")

Exit codes: 0 imported (or a dry run), 1 refused (no semantic-release
configuration, a JavaScript one, engine keys that differ without --force,
a result that does not load), 2 usage or environment error.
`

// runReleaseSetup is `nerv release setup`.
func runReleaseSetup(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("release setup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cwd := fs.String("cwd", ".", "")
	doImport := fs.Bool("import", false, "")
	dryRun := fs.Bool("dry-run", false, "")
	force := fs.Bool("force", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, releaseSetupUsage)
			return 0
		}
		return releaseSetupUsageError(stderr, err.Error())
	}
	if fs.NArg() > 0 {
		return releaseSetupUsageError(stderr, fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}
	if !*doImport {
		return releaseSetupUsageError(stderr, "--import is required (it is the only setup mode; the build settings are written by /release:setup)")
	}
	err := releaseSetupImport(context.Background(), *cwd, *dryRun, *force, stdout, stderr, opts)
	if err == nil {
		return 0
	}
	fmt.Fprintf(stderr, "nerv release setup: %v\n", err)
	var invalid *releaseconfig.ValidationError
	if refusal.Is(err) || errors.As(err, &invalid) {
		return 1
	}
	return exitCodeFor(err)
}

func releaseSetupUsageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "nerv release setup: %s\n\n%s", problem, releaseSetupUsage)
	return 2
}

func releaseSetupImport(ctx context.Context, cwd string, dryRun, force bool, stdout, stderr io.Writer, opts options) error {
	top, err := gitTopLevel(ctx, opts.Runner, cwd)
	if err != nil {
		return err
	}
	path := paths.ProjectConfig(top)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	imported, err := versioning.Import(top)
	var manual *versioning.ManualMigrationError
	switch {
	case errors.As(err, &manual):
		return &refusal.Error{Err: fmt.Errorf("%w; write the release: section's branches and release_rules from it (see /release:setup)", err)}
	case errors.Is(err, versioning.ErrNoSemanticReleaseConfig):
		return &refusal.Error{Err: fmt.Errorf("%v in %s (package.json \"release\", .releaserc, .releaserc.json, .releaserc.yaml or .yml): nothing to import", err, top)}
	case err != nil:
		return &refusal.Error{Err: err}
	}
	text, notes, err := releaseconfig.ImportSection(data, imported.Config, force)
	var conflict *releaseconfig.EngineConflictError
	if errors.As(err, &conflict) {
		return &refusal.Error{Err: fmt.Errorf("%w; nothing was written: keep them, edit them by hand, or run again with --force to replace them with the imported values", err)}
	}
	if err != nil {
		return err
	}
	for _, w := range imported.Warnings {
		fmt.Fprintf(stderr, "nerv release setup: warning: %s\n", w)
	}
	for _, n := range notes {
		fmt.Fprintf(stderr, "nerv release setup: %s\n", n)
	}
	if dryRun {
		block, _ := config.Parse(text).Block(releaseconfig.SectionKey)
		fmt.Fprintf(stdout, "%s\n", block)
		fmt.Fprintf(stderr, "nerv release setup: dry run: %s was not written (imported from %s)\n", path, imported.Source)
		return nil
	}
	if err := atomicfile.Write(path, []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "nerv release setup: imported %s into the release: section of %s (%d warning(s))\n", imported.Source, path, len(imported.Warnings))
	return nil
}
