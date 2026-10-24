package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

const migrateUsage = `Usage: nerv migrate [--user] [--dry-run] [--install] [--force] [--cwd <dir>]

Moves a repository (and, with --user, your user configuration) from nerv
4.2.0 to this version, once:

  - .nerv/nerv.yaml becomes .ai-workflow.yml, and ~/.claude/nerv/nerv.yaml
    (with --user) becomes the user config.yml; comments and key order are
    kept, and each old file stays as <file>.migrated so an older nerv does
    not read stale configuration;
  - personal settings in the project file (models, and every provider key
    of user scope) move to the user config with --user, and are reported
    otherwise;
  - tasks.rounding_minutes moves to tasks.providers.teamwork, and the
    removed jira and github-projects providers are dropped;
  - release: becomes ship:, each project's presets become build.mode:
    script with the commands 4.2.0 ran (a TODO(nerv migrate) comment marks
    what has no exact equivalent), and the semantic-release configuration
    is imported (as nerv ship setup --import does);
  - pipeline: becomes deploy: (one deploy-ssh-compose target per
    environment), with deploy.host github when origin is on GitHub;
  - each in-flight FULL openspec/changes/<change>/ becomes
    odd/tasks/<change>.md and its run files odd/<change>/nerv/; a DIRECT or
    LIGHT one moves whole to .nerv/runs/<change>/ with no ODD document; the
    lock moves to .nerv/locks/<change>.lock; a change whose lock is held on
    this machine (a heartbeat under 15 minutes old, or a run waiting on
    you) is refused unless --force; archived changes and openspec/specs
    stay; strict_tdd becomes odd.tdd;
  - .nerv/.gitignore is created.

It prints every change (a unified diff per file), what has no equivalent,
and the nerv install line for the plugins the repository used. Every file
it changes or removes is backed up first (the repository's under
.nerv/backups/migrate-<timestamp>/, the user config's next to it). When a
step fails, every change already made is rolled back. A run interrupted
before it could roll back is finished by the next one. A second run changes
nothing.

Flags:
  --dry-run      Print the changes; write nothing
  --user         Also migrate the user configuration (alone when the
                 directory is not inside a git repository)
  --install      Then install the plugins the repository used (nerv install
                 --with ..., which also offers to remove old /task:* copies)
  --force        Move a change whose orchestrator lock is still held
  --cwd <dir>    A directory inside the repository (default ".")

Exit codes: 0 migrated, nothing to migrate, or a dry run; 1 refused (not a
repository, both the old and the new file exist, a result that does not
load, a lock still held); 2 usage or environment error.
`

func runMigrate(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cwd := fs.String("cwd", ".", "")
	dryRun := fs.Bool("dry-run", false, "")
	user := fs.Bool("user", false, "")
	install := fs.Bool("install", false, "")
	force := fs.Bool("force", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, migrateUsage)
			return 0
		}
		fmt.Fprintf(stderr, "nerv migrate: %v\n\n%s", err, migrateUsage)
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "nerv migrate: unexpected argument %q\n\n%s", fs.Arg(0), migrateUsage)
		return 2
	}
	err := migrateRun(context.Background(), *cwd, *dryRun, *user, *install, *force, stdout, stderr, opts)
	if err == nil {
		return 0
	}
	var code *exitError
	if errors.As(err, &code) {
		return code.code
	}
	fmt.Fprintf(stderr, "nerv migrate: %v\n", err)
	return exitCodeFor(err)
}

// exitError carries the exit code of a step that printed its own error.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func migrateRun(ctx context.Context, cwd string, dryRun, user, install, force bool, stdout, stderr io.Writer, opts options) error {
	home, err := configure.ResolveHome(opts.Home)
	if err != nil {
		return err
	}
	top, err := gitTopLevel(ctx, opts.Runner, cwd)
	if err != nil {
		if !user || !refusal.Is(err) {
			return err
		}
		top = "" // --user outside a repository: the user configuration only.
	}
	o := migrate.Options{RepoRoot: top, Home: home, User: user, ExtensionsFS: opts.ExtensionsFS, Now: opts.Now(), Force: force}
	if top != "" {
		o.OriginURL = originURL(ctx, opts, top)
	}
	plan, err := migrate.Compute(o)
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, migrate.Render(plan))
	for _, n := range plan.Notes {
		fmt.Fprintf(stdout, "note: %s\n", n)
	}
	for _, w := range plan.Warnings {
		fmt.Fprintf(stdout, "warning: %s\n", w)
	}
	line := plan.InstallCommand()
	if line != "" {
		fmt.Fprintf(stdout, "plugins: %s\n", line)
	}
	switch {
	case plan.Empty():
		fmt.Fprintln(stdout, "nerv migrate: nothing to migrate")
	case dryRun:
		fmt.Fprintf(stdout, "nerv migrate: dry run: %d change(s), nothing was written\n", len(plan.Changes))
		return nil
	default:
		backups, err := migrate.Apply(plan, o.Now)
		for _, b := range backups {
			fmt.Fprintf(stdout, "backup: %s\n", b)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "nerv migrate: %d change(s) written\n", len(plan.Changes))
	}
	if install && line != "" && !dryRun {
		with := strings.TrimPrefix(line, "nerv install --with ")
		if code := runInstall([]string{"--with", with, "--no-configure"}, stdout, stderr, opts); code != 0 {
			return &exitError{code}
		}
	}
	return nil
}

// originURL is the URL of the repository's origin remote ("" for none).
func originURL(ctx context.Context, opts options, top string) string {
	out, _, code, err := opts.Runner.Run(ctx, "git", "-C", top, "remote", "get-url", "origin")
	if err != nil || code != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}
