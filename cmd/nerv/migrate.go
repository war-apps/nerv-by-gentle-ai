package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/ecosystem"
	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-kit-by-gentle-ai/aiworkflow"
)

const migrateUsage = `Usage: nerv migrate [--user] [--dry-run] [--install] [--force] [--cwd <dir>]

Moves a repository (and, with --user, your user configuration) from nerv
4.2.0 or 5.0.0-alpha.1 to this version, once:

  - .nerv/nerv.yaml (4.2.0) or .ai-workflow.yml (5.0.0-alpha.1) becomes
    .ai-workflow/config.yaml, and ~/.claude/nerv/nerv.yaml or the user
    config.yml (with --user) becomes the user config.yaml; comments and key
    order are kept; .ai-workflow.yml is removed after its backup, and every
    other old file stays as <file>.migrated so an older nerv does not read
    stale configuration;
  - personal settings in the project file (models, and every provider key
    of user scope) move to the user config with --user, and are reported
    otherwise;
  - with --user, the user config's tasks:, release: and deploy: keys,
    which apply from the project file only, move under defaults.<section>
    (the defaults the setup commands copy into a project); the personal
    ones stay: provider keys of user scope, tasks.timer_store,
    tasks.stale_after_hours and tasks.rounding_minutes;
  - a project file without tasks.provider gets the user's
    (defaults.tasks.provider, or the user tasks.provider), so a repository
    that relied on it keeps its provider;
  - tasks.rounding_minutes moves to tasks.providers.teamwork, and the
    removed jira and github-projects providers are dropped;
  - an older nerv's release and deploy configuration (a 4.2.0 release:
    or pipeline: section, the 5.0.0-alpha.1 ship: section, a 5.x deploy
    target with provider: bash, or a deploy-<name> project provider) is
    converted by nerv-release: when nerv-release is installed, nerv
    migrate moves those sections, as they are, to
    .ai-workflow/release.yaml (the user config's to the global
    release.yaml, with --user), leaves a deploy-<name> provider where it
    is, and, once its steps are written, runs nerv-release migrate --cwd
    <repository> (and nerv-release migrate --user), which converts them;
    without nerv-release, nerv migrate refuses, naming the file, the
    section and nerv install --tools nerv-release;
  - when nerv-task is installed, tasks: moves to nerv-task's own files
    instead: the project file's tasks: to .ai-workflow/task.yaml (repo
    keys), and, with --user, the user config's personal tasks: keys and
    its defaults.tasks: seed to the global task.yaml (keys without the
    "tasks."/"defaults.tasks." prefix); a key already set in task.yaml is
    never overwritten, and is reported instead;
  - when nerv-release is installed, release: and deploy: move to
    nerv-release's own files, every key at the same path: the project
    file's release: and deploy: to .ai-workflow/release.yaml, and, with
    --user, the user config's personal deploy keys
    (deploy.providers.ssh-compose.infisical_domain, ssh_auth,
    ssh_key_file, ssh_known_hosts, ssh_connect_timeout) and its
    defaults.release:/defaults.deploy: seeds to the global release.yaml;
    a personal key in the project file is never moved, and is reported;
  - each in-flight FULL openspec/changes/<change>/ becomes
    odd/tasks/<change>.md and its run files odd/<change>/nerv/; a DIRECT or
    LIGHT one moves whole to .nerv/runs/<change>/ with no ODD document; the
    lock moves to .nerv/locks/<change>.lock; a change whose lock is held on
    this machine (a heartbeat under 15 minutes old, or a run waiting on
    you) is refused unless --force; archived changes and openspec/specs
    stay; strict_tdd becomes odd.tdd;
  - .nerv/.gitignore is created.

It lists its steps, with or without --dry-run: each file it creates,
updates, moves or deletes, and what for; then how the configuration
changes, what to check by hand, and the nerv install line for the
tools the repository uses. Every file
it changes or removes is backed up first (the repository's under
.nerv/backups/migrate-<timestamp>/, the user config's next to it). When a
step fails, every change already made is rolled back. A run interrupted
before it could roll back is finished by the next one. A second run changes
nothing.

Flags:
  --dry-run      List the steps; write nothing
  --user         Also migrate the user configuration (alone when the
                 directory is not inside a git repository)
  --install      Then run nerv install --no-configure, with --tools naming
                 the NERV-ecosystem tools that replace the plugins the
                 repository used (it also offers to remove old /task:* copies)
  --force        Move a change whose orchestrator lock is still held
  --cwd <dir>    A directory inside the repository (default ".")

A dry run does not run nerv-release migrate: it says it would.

Exit codes: 0 migrated, nothing to migrate, or a dry run; 1 refused (not a
repository, both the old and the new file exist, an older release or
deploy section without nerv-release, nerv-release migrate refusing the
sections moved to release.yaml, a lock still held); 2 usage or
environment error (nerv-release migrate failing or not starting too).
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

// taskYAMLTool is the name nerv-task resolves its own files under (its
// own internal/config.Tool), used here only to compute where those files
// live (kit aiworkflow.UserConfigPath/ProjectConfigPath): nerv never
// imports nerv-task's module.
const taskYAMLTool = "task"

// releaseYAMLTool is the name nerv-release resolves its own files under
// (release.yaml), used here only to compute where those files live.
const releaseYAMLTool = "release"

// catalogueToolPath is where the ecosystem catalogue tool named name
// (S12) is installed on this machine: on PATH, or in its default install
// directory (internal/ecosystem's own detection); "" when it is not. A
// name the catalogue does not list is never installed.
func catalogueToolPath(ctx context.Context, opts options, home, name string) string {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	for _, t := range ecosystem.Catalogue() {
		if t.Name != name {
			continue
		}
		return ecosystem.Detect(ctx, t, opts.Runner, opts.LookPath, goos, home, opts.Getenv).Path
	}
	return ""
}

// setToolFileTargets points o at the own files of every catalogue tool
// installed (S2, S12): nerv-task's task.yaml and nerv-release's
// release.yaml, the repository's when o.RepoRoot is set and, with o.User,
// the global ones. A tool not installed leaves its targets "", so its
// sections stay in config.yaml, unchanged. nerv migrate,
// nerv doctor and the install/upgrade offer all build their options here.
// It returns where nerv-release is installed ("" when it is not), for
// nerv migrate to run nerv-release migrate.
func setToolFileTargets(ctx context.Context, opts options, home string, o *migrate.Options) (nervRelease string) {
	// The global files sit next to the user config.yaml nerv migrates, in
	// the directory paths.Resolve already chose: the real environment
	// (XDG_CONFIG_HOME, %AppData%) applies to the real home only, never to
	// a fake one.
	global := func(tool string) string {
		return filepath.Join(filepath.Dir(paths.Resolve(home).UserConfig), tool+".yaml")
	}
	if catalogueToolPath(ctx, opts, home, "nerv-task") != "" {
		if o.RepoRoot != "" {
			o.TaskYAMLRepo = aiworkflow.ProjectConfigPath(o.RepoRoot, taskYAMLTool)
		}
		if o.User {
			o.TaskYAMLHome = global(taskYAMLTool)
		}
	}
	nervRelease = catalogueToolPath(ctx, opts, home, "nerv-release")
	if nervRelease != "" {
		if o.RepoRoot != "" {
			o.ReleaseYAMLRepo = aiworkflow.ProjectConfigPath(o.RepoRoot, releaseYAMLTool)
		}
		if o.User {
			o.ReleaseYAMLHome = global(releaseYAMLTool)
		}
	}
	return nervRelease
}

// releaseMigrateRun is one nerv-release migrate run: its arguments, and
// the release.yaml that holds the sections it converts.
type releaseMigrateRun struct {
	args  []string
	where string
}

// releaseMigrateRuns are the nerv-release migrate runs plan hands an
// older nerv's release and deploy configuration to (S5): the
// repository's (--cwd <top>), with --user when nerv migrate runs with
// --user (it then also moves a converted pipeline:'s personal keys to the
// global release.yaml and converts that file, in the same run); or the
// user configuration's alone (--user, with --cwd <top> inside a
// repository).
func releaseMigrateRuns(plan *migrate.Plan, o migrate.Options) []releaseMigrateRun {
	var runs []releaseMigrateRun
	// A section moved to a release.yaml runs it too: nerv does not tell
	// every older shape apart (a release: whose engine keys sit at its
	// top), and a current section leaves it nothing to do.
	writes := func(path string) bool {
		return path != "" && slices.ContainsFunc(plan.Changes, func(c migrate.Change) bool { return c.Path == path })
	}
	if plan.LegacyReleaseRepo || writes(o.ReleaseYAMLRepo) {
		args := []string{"migrate", "--cwd", o.RepoRoot}
		if o.User {
			args = append(args, "--user")
		}
		return append(runs, releaseMigrateRun{args, o.ReleaseYAMLRepo})
	}
	if plan.LegacyReleaseUser || writes(o.ReleaseYAMLHome) {
		args := []string{"migrate"}
		if o.RepoRoot != "" {
			args = append(args, "--cwd", o.RepoRoot)
		}
		runs = append(runs, releaseMigrateRun{append(args, "--user"), o.ReleaseYAMLHome})
	}
	return runs
}

// line is the run as a user types it.
func (r releaseMigrateRun) line() string { return "nerv-release " + strings.Join(r.args, " ") }

// runReleaseMigrate runs nerv-release (at binary) for r, printing its
// output, and maps its exit code (S5): 1, nerv-release migrate refusing
// the sections nerv migrate moved to r.where, is a refusal (exit 1); any
// other failure, or a run that could not start, is exit 2.
func runReleaseMigrate(ctx context.Context, opts options, binary string, r releaseMigrateRun, stdout, stderr io.Writer) error {
	fmt.Fprintf(stdout, "\nnerv migrate: running %s\n", r.line())
	out, errOut, code, err := opts.Runner.Run(ctx, binary, r.args...)
	fmt.Fprint(stdout, out)
	fmt.Fprint(stderr, errOut)
	switch {
	case err != nil:
		fmt.Fprintf(stderr, "nerv migrate: %s could not run: %v\n", r.line(), err)
		return &exitError{2}
	case code == 0:
		return nil
	case code == 1:
		fmt.Fprintf(stderr, "nerv migrate: the older release and deploy sections are in %s, moved as they were, but nerv-release migrate refused to convert them: fix what it reports, then run %s\n", r.where, r.line())
		return &exitError{1}
	default:
		fmt.Fprintf(stderr, "nerv migrate: %s failed (exit %d)\n", r.line(), code)
		return &exitError{2}
	}
}

// toolFileMove is one tool's own file a plan writes: a task.yaml or
// release.yaml move still pending (S3).
type toolFileMove struct {
	Tool, Path string
}

// pendingToolFileMoves is the tool files plan writes, of the targets o
// set (setToolFileTargets), nerv-task's before nerv-release's.
func pendingToolFileMoves(plan *migrate.Plan, o migrate.Options) []toolFileMove {
	targets := []toolFileMove{
		{"nerv-task", o.TaskYAMLRepo}, {"nerv-task", o.TaskYAMLHome},
		{"nerv-release", o.ReleaseYAMLRepo}, {"nerv-release", o.ReleaseYAMLHome},
	}
	var moves []toolFileMove
	for _, target := range targets {
		if target.Path == "" {
			continue
		}
		for _, c := range plan.Changes {
			if c.Path == target.Path {
				moves = append(moves, target)
				break
			}
		}
	}
	return moves
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
	o := migrate.Options{RepoRoot: top, Home: home, User: user, Now: opts.Now(), Force: force}
	nervRelease := setToolFileTargets(ctx, opts, home, &o)
	plan, err := migrate.Compute(o)
	if err != nil {
		return err
	}
	handOff := releaseMigrateRuns(plan, o)
	switch {
	case plan.Empty() && (!dryRun || len(handOff) == 0):
		fmt.Fprint(stdout, migrate.Render(plan))
		fmt.Fprintln(stdout, "nerv migrate: nothing to migrate")
	case dryRun:
		fmt.Fprintln(stdout, "nerv migrate --dry-run: these are the steps nerv migrate would take; nothing is written:")
		fmt.Fprint(stdout, migrate.Render(plan))
		for _, r := range handOff {
			fmt.Fprintf(stdout, "\nnerv migrate would then run %s, which converts them\n", r.line())
		}
		fmt.Fprintf(stdout, "\nnerv migrate: dry run: %d change(s), nothing was written\n", len(plan.Changes))
		return nil
	default:
		fmt.Fprintln(stdout, "nerv migrate: these are the steps:")
		fmt.Fprint(stdout, migrate.Render(plan))
		backups, err := migrate.Apply(plan, o.Now)
		if len(backups) > 0 {
			fmt.Fprintln(stdout, "\nBackups:")
			for _, b := range backups {
				fmt.Fprintf(stdout, "  - %s\n", b)
			}
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "\nnerv migrate: %d change(s) written\n", len(plan.Changes))
	}
	for _, r := range handOff {
		if err := runReleaseMigrate(ctx, opts, nervRelease, r, stdout, stderr); err != nil {
			return err
		}
	}
	if install && len(plan.Tools) > 0 && !dryRun {
		args := []string{"--tools", strings.Join(plan.Tools, ","), "--no-configure"}
		if code := runInstall(args, stdout, stderr, opts); code != 0 {
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
