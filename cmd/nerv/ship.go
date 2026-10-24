package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipclone"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipplan"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

const shipUsage = `Usage: nerv ship [flags]

Builds, tests and packages each project of the ship: section of
.ai-workflow.yml from your machine, as the CI build would, and leaves a local
artifact for nerv deploy. The version is computed natively (see "nerv
version next --help"), or given with --version. In a fresh temporary clone
of origin's branch, for
each project with a new release in dependency order, it writes the version
into build.version_files and the CHANGELOG section into the project's
CHANGELOG (in the clone only), runs the pipeline of the project's
build.mode (compose or script: restore, lint, build, test, coverage,
package) and the prepare hooks, then writes the artifact
.nerv/artifacts/<project>/<version>/ in your repository: manifest.json, the
notes, the CHANGELOG section, the outputs and the changed release files.

nerv ship never writes to git: no commit, no tag, no push; nerv deploy does
that from the artifact. Your repository is never modified (only its
.nerv/artifacts/, which ignores itself), and only what origin has is built:
a local branch ahead of or behind origin is refused. The ship: section is
your local .ai-workflow.yml, committed or not; the code is origin's.

Flags:
  --cwd <dir>              A directory inside the repository (default ".")
  --branch <name>          Branch to build (default: the current one)
  --project <name>         Only this project of the ship: section
  --version <semver>       Use this version instead of computing it (with
                            several projects, --project is required); it
                            must fit the branch and be newer than the
                            project's last release on it (as the engine
                            picks it), its channel is the branch's, and its
                            notes cover the commits since that release
  --dry-run                Run the pipeline and the prepare hooks in the
                            clone, but write no artifact
  --force                  Replace an existing artifact of the same version
  --yes                    Package without asking for confirmation
  --fetch-timeout <dur>    Bound for each read of origin (default 60s)
  --expect <project>=<version>
                           Package only if the versions are exactly these
                            (repeat once per releasing project, as the dry
                            run printed them); anything else is refused
                            before any stage runs
  --json                   Print the final summary as JSON

Progress goes to stderr, the final summary to stdout. Exit codes: 0
packaged (also when there is nothing to release, and a dry run), 1 refused
(invalid configuration, an old release: or pipeline: section, unknown
project, local branch not equal to origin's, an existing artifact without
--force, no confirmation), 2 a step or the environment failed, 128 + the
signal when interrupted.

  nerv ship setup --import [--dry-run]   imports a semantic-release
                                         configuration (see "nerv ship
                                         setup --help")
`

// shipWorkspace is the release clone nerv ship works in; *shipclone.Workspace
// is the real one.
type shipWorkspace interface {
	Dir() string
	// Config is the user's local ship: section (nil when none), the one the
	// build uses; the clone's copy is ignored.
	Config() *shipconfig.Release
	ArtifactsDir() string
	NotesDir() string
	OriginHead() string
	PublicURL() string
	// Compute computes the projects' next releases in the clone, offline.
	Compute(ctx context.Context, projects []string) ([]versioning.Result, error)
	// Explicit is the project's release at the explicit version, from
	// the engine in the clone: its last release on the branch and the
	// version's notes and CHANGELOG section.
	Explicit(ctx context.Context, project, version string) (versioning.Result, error)
	// Branch resolves the branch against the ship: section's branches.
	Branch(ctx context.Context) (versioning.ReleaseBranch, error)
	BranchOnOrigin(ctx context.Context) (string, error)
	Close() error
}

// Statuses of a project in the final summary.
const (
	shipPackaged   = "packaged"
	shipDryRun     = "dry-run"
	shipNoRelease  = "no-release"
	shipFailed     = "failed"
	shipNotStarted = "not-started"
)

// shipOutcome is what happened to one project (the --json summary).
type shipOutcome struct {
	Project     string `json:"project"`
	Branch      string `json:"branch"`
	Status      string `json:"status"`
	Version     string `json:"version,omitempty"`
	Channel     string `json:"channel,omitempty"`
	Tag         string `json:"tag,omitempty"`
	LastVersion string `json:"last_version,omitempty"`
	// Artifact is the artifact directory written (status packaged).
	Artifact string `json:"artifact,omitempty"`
	Error    string `json:"error,omitempty"`
	// Action is what the user must do after a failure.
	Action string `json:"action,omitempty"`
}

// shipRun is one nerv ship invocation.
type shipRun struct {
	opts           options
	stdout, stderr io.Writer
	cwd, branch    string
	project        string
	dryRun, yes    bool
	timeout        time.Duration
	expect         expectations
	// version is --version (empty: computed); force is --force.
	version string
	force   bool

	top  string
	ws   shipWorkspace
	exec shipplan.Executor
	// configSource is where the computed versions' configuration came from
	// (configRepository: the ship: section); empty with --version.
	configSource string
}

func runShip(args []string, stdout, stderr io.Writer, opts options) int {
	if len(args) > 0 && args[0] == "setup" {
		return runShipSetup(args[1:], stdout, stderr, opts)
	}
	fs := flag.NewFlagSet("ship", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	s := &shipRun{opts: opts, stdout: stdout, stderr: stderr}
	fs.StringVar(&s.cwd, "cwd", ".", "")
	fs.StringVar(&s.branch, "branch", "", "")
	fs.StringVar(&s.project, "project", "", "")
	fs.StringVar(&s.version, "version", "", "")
	fs.BoolVar(&s.dryRun, "dry-run", false, "")
	fs.BoolVar(&s.force, "force", false, "")
	fs.BoolVar(&s.yes, "yes", false, "")
	fs.DurationVar(&s.timeout, "fetch-timeout", shipclone.DefaultFetchTimeout, "")
	s.expect = expectations{}
	fs.Var(s.expect, "expect", "")
	jsonOut := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, shipUsage)
			return 0
		}
		return shipUsageError(stderr, err.Error())
	}
	if fs.NArg() > 0 {
		return shipUsageError(stderr, fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}
	if s.timeout <= 0 {
		return shipUsageError(stderr, fmt.Sprintf("--fetch-timeout %s: must be positive", s.timeout))
	}
	if s.version != "" && !shipplan.ValidVersion(s.version) {
		return shipUsageError(stderr, fmt.Sprintf("--version %q: not a semantic version such as 1.4.0 or 1.4.0-rc.1", s.version))
	}

	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop, received := signalCtx(context.Background())
	defer stop()

	outcomes, err := s.execute(ctx)
	doc := shipDocument{Status: successStatus(outcomes, s.dryRun), Projects: outcomes, SemanticReleaseConfig: s.configSource}
	code := 0
	if err != nil {
		code, doc.Status, doc.Error = classifyShipError(ctx, received(), stderr, err)
	}
	if doc.Projects == nil {
		doc.Projects = []shipOutcome{}
	}
	if *jsonOut || outcomes != nil {
		if werr := writeShipSummary(stdout, *jsonOut, s.dryRun, doc); werr != nil && code == 0 {
			fmt.Fprintf(stderr, "nerv ship: writing the summary: %v\n", werr)
			code = 2
		}
	}
	return code
}

// shipDocument is the --json summary, printed on every run that gets past
// the flags: the run's status (packaged, dry-run, nothing-to-release,
// refused, declined, failed, interrupted), its error, and each project's
// outcome (empty when the run stopped before computing them).
type shipDocument struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// SemanticReleaseConfig is where the version configuration that
	// computed the versions came from: "repository" (the ship: section);
	// empty with --version and before the versions are computed. The name
	// is kept from the semantic-release engine.
	SemanticReleaseConfig string        `json:"semantic_release_config,omitempty"`
	Projects              []shipOutcome `json:"projects"`
}

// successStatus is the status of a run that ended without an error.
func successStatus(outcomes []shipOutcome, dryRun bool) string {
	if !slices.ContainsFunc(outcomes, func(o shipOutcome) bool { return o.Status != shipNoRelease }) {
		return "nothing-to-release"
	}
	if dryRun {
		return "dry-run"
	}
	return shipPackaged
}

// errDeclined is the answer "no" to the confirmation.
var errDeclined = errors.New("cancelled: nothing was released")

// errShipDeclined is the answer "no" to nerv ship's confirmation.
var errShipDeclined = fmt.Errorf("%w (no artifact was written)", errDeclined)

// classifyShipError prints err on stderr and returns the exit code, the run
// status and the error text: 128 + the signal (interrupted), 1 for a
// refusal or an invalid configuration (declined when the user said no), 2
// otherwise (failed).
func classifyShipError(ctx context.Context, sig os.Signal, stderr io.Writer, err error) (int, string, string) {
	if ctx.Err() != nil {
		if sig != nil {
			fmt.Fprintf(stderr, "nerv ship: interrupted (%v)\n", sig)
			return interruptedExitCode(sig), "interrupted", "interrupted (" + sig.String() + ")"
		}
		fmt.Fprintln(stderr, "nerv ship: interrupted")
		return 2, "interrupted", "interrupted"
	}
	fmt.Fprintf(stderr, "nerv ship: %v\n", err)
	var invalid *shipconfig.ValidationError
	switch {
	case errors.Is(err, errDeclined):
		return 1, "declined", err.Error()
	case errors.As(err, &invalid), exitCodeFor(err) == 1:
		return 1, "refused", err.Error()
	}
	return 2, "failed", err.Error()
}

// expectations are the --expect <project>=<version> pairs: the versions the
// user approved (from a dry run). An empty set checks nothing.
type expectations map[string]string

// expectProjectRe is a ship: project name (shipconfig: [a-z0-9-]) or a
// repository directory name for a sectionless repository.
var expectProjectRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// expectVersionRe is a semantic version's characters.
var expectVersionRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]*$`)

func (e expectations) String() string { return "" }

// Set parses one <project>=<version>; a project given twice is an error.
func (e expectations) Set(v string) error {
	name, version, ok := strings.Cut(v, "=")
	if !ok || !expectProjectRe.MatchString(name) || !expectVersionRe.MatchString(version) {
		return fmt.Errorf("want <project>=<version>, got %q", v)
	}
	if _, dup := e[name]; dup {
		return fmt.Errorf("project %q given twice", name)
	}
	e[name] = version
	return nil
}

// check refuses unless the releasing projects and their versions (project
// -> version) are exactly the expected ones: no other version, no missing
// release, no release that was not expected. explicit says the versions come
// from --version, not from a computation.
func (e expectations) check(computed map[string]string, explicit bool) error {
	if len(e) == 0 {
		return nil
	}
	var problems []string
	for _, name := range slices.Sorted(maps.Keys(e)) {
		switch got, ok := computed[name]; {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: expected %s, computed no release", name, e[name]))
		case got != e[name]:
			problems = append(problems, fmt.Sprintf("%s: expected %s, computed %s", name, e[name], got))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(computed)) {
		if _, ok := e[name]; !ok {
			problems = append(problems, fmt.Sprintf("%s: not expected, computed %s", name, computed[name]))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	if explicit {
		return refuse("--version does not match --expect (%s); nothing was run. Pass the same version to both, as the dry run printed it",
			strings.Join(problems, "; "))
	}
	return refuse("the computed release is not the expected one (%s); nothing was run. Origin probably moved since the dry run: run the dry run again and review the new plan",
		strings.Join(problems, "; "))
}

func shipUsageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "nerv ship: %s\n\n%s", problem, shipUsage)
	return 2
}

func refuse(format string, args ...any) error {
	return &refusal.Error{Err: fmt.Errorf(format, args...)}
}

// execute builds and returns the projects' outcomes (nil when it stopped
// before knowing them).
func (s *shipRun) execute(ctx context.Context) ([]shipOutcome, error) {
	top, err := gitTopLevel(ctx, s.opts.Runner, s.cwd)
	if err != nil {
		return nil, err
	}
	s.top = top
	// Refuse an invalid configuration or an unknown --project before
	// reading origin.
	if _, err := requestShape(top, s.project); err != nil {
		return nil, err
	}
	if err := s.checkShipConfig(top); err != nil {
		return nil, err
	}
	local, err := s.localBranch(ctx, top)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(s.stderr, "nerv ship: reading origin's %s into a release clone of %s\n", s.branch, top)
	open := s.opts.OpenShipWorkspace
	if open == nil {
		open = func(ctx context.Context, o shipclone.Options) (shipWorkspace, error) {
			return shipclone.New(ctx, o)
		}
	}
	ws, err := open(ctx, shipclone.Options{RepoRoot: top, Branch: s.branch, FetchTimeout: s.timeout})
	if errors.Is(err, shipclone.ErrNoOrigin) {
		return nil, refuse("the repository has no origin remote: nerv ship builds what is on origin")
	}
	if err != nil {
		return nil, err
	}
	s.ws = ws
	defer func() {
		if err := ws.Close(); err != nil {
			fmt.Fprintf(s.stderr, "nerv ship: removing the release clone %s: %v\n", ws.Dir(), err)
		}
	}()
	fmt.Fprintf(s.stderr, "nerv ship: release clone %s (removed at the end)\n", ws.Dir())

	switch head := ws.OriginHead(); {
	case head == "":
		return nil, refuse("branch %s is not on origin: push it first (nerv ship builds what is on origin)", s.branch)
	case head != local:
		return nil, refuse("your local %s (%s) is not origin/%s (%s): nerv ship builds what is on origin and never pushes your commits; push or pull first",
			s.branch, short(local), s.branch, short(head))
	}

	// Refuse a local section naming paths origin's branch lacks before
	// anything is planned with it.
	if err := checkPathsOnOrigin(ws.Config(), ws.Dir(), s.branch); err != nil {
		return nil, err
	}
	return s.shipProjects(ctx)
}

// checkShipConfig refuses, before origin is read, a repository nerv ship
// cannot build: no ship: section, a project without build.mode, --version
// with several projects and no --project, or a .nerv/artifacts that leads
// outside the repository (before any stage runs).
func (s *shipRun) checkShipConfig(top string) error {
	rel, err := shipconfig.Load(top)
	if errors.Is(err, shipconfig.ErrNotConfigured) {
		return refuse("%s", withLegacyHint("the repository has no ship: section in .ai-workflow.yml: nerv ship needs one, with build.mode (compose or script) for each project; see /ship:setup", top))
	}
	if err != nil {
		return err
	}
	if err := rel.CheckBuild(); err != nil {
		return err
	}
	if s.version != "" && s.project == "" && len(rel.Projects) > 1 {
		return refuse("--version %s: the ship: section declares %d projects, each with its own version: pass --project too", s.version, len(rel.Projects))
	}
	if !s.dryRun {
		if err := artifact.CheckRoot(top); err != nil {
			return refuse("%v (nothing was run)", err)
		}
	}
	return nil
}

// localBranch resolves the branch to build (default: the current one) and
// returns its local commit.
func (s *shipRun) localBranch(ctx context.Context, top string) (string, error) {
	if s.branch == "" {
		out, _, code, err := s.opts.Runner.Run(ctx, "git", "-C", top, "symbolic-ref", "--short", "HEAD")
		if err != nil {
			return "", fmt.Errorf("git symbolic-ref: %w", err)
		}
		if code != 0 || strings.TrimSpace(out) == "" {
			return "", refuse("the checkout is not on a branch: pass --branch")
		}
		s.branch = strings.TrimSpace(out)
	}
	out, _, code, err := s.opts.Runner.Run(ctx, "git", "-C", top, "rev-parse", "--verify", "--quiet", "refs/heads/"+s.branch)
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	if code != 0 || strings.TrimSpace(out) == "" {
		return "", refuse("branch %q is not a local branch", s.branch)
	}
	return strings.TrimSpace(out), nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// checkPathsOnOrigin refuses when a path the local ship: section names
// (project path, shared path without glob characters, compose file,
// version file, stage and hook workdirs) does not exist in the release
// clone: the configuration is the user's local one, but the code is
// origin's branch.
func checkPathsOnOrigin(rel *shipconfig.Release, clone, branch string) error {
	if rel == nil {
		return nil
	}
	var missing []string
	check := func(project, field, path string) {
		if path == "" || path == "." || strings.ContainsAny(path, "*?[{") {
			return
		}
		// Joining is safe: shipconfig validation rejects absolute paths and
		// paths escaping the repository root, and every path in the section
		// is relative to that root (validate.go checkPath), so path stays
		// inside the clone.
		if _, err := os.Stat(filepath.Join(clone, filepath.FromSlash(path))); err != nil {
			missing = append(missing, fmt.Sprintf("%q (project %s, %s)", path, project, field))
		}
	}
	for _, p := range rel.Projects {
		check(p.Name, "path", p.Path)
		for _, sp := range p.SharedPaths {
			check(p.Name, "shared_paths", sp)
		}
		if b := p.Build; b != nil {
			check(p.Name, "build.file", b.File)
			for _, f := range b.VersionFiles {
				check(p.Name, "build.version_files", f)
			}
		}
		for _, stage := range slices.Sorted(maps.Keys(p.Stages)) {
			check(p.Name, "stages."+stage+".workdir", p.Stages[stage].Workdir)
		}
		for _, key := range slices.Sorted(maps.Keys(p.Hooks)) {
			for _, c := range p.Hooks[key] {
				check(p.Name, "hooks."+key+".workdir", c.Workdir)
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return refuse("the ship: section names paths that are not on origin's %s: %s (the configuration is your local one, the code is origin's branch: push them, or fix the section)",
		branch, strings.Join(missing, "; "))
}

// shipItem is one releasing project with its version, configuration and
// plan.
type shipItem struct {
	version shipplan.Version
	project shipconfig.Project
	plan    *shipplan.Plan
	outcome *shipOutcome
}

// shipProjects gets each project's version, plans every releasing one,
// shows the plans, asks for confirmation and builds them in dependency
// order, stopping at the first failure.
func (s *shipRun) shipProjects(ctx context.Context) ([]shipOutcome, error) {
	rel := s.ws.Config()
	if rel == nil {
		return nil, refuse("the repository has no ship: section in .ai-workflow.yml")
	}
	ordered, err := rel.TopoOrder()
	if err != nil {
		return nil, err
	}
	ordered = slices.DeleteFunc(ordered, func(p shipconfig.Project) bool { return s.project != "" && p.Name != s.project })
	src := s.versionSource()
	versions := make([]shipplan.Version, len(ordered))
	releasing := map[string]string{}
	for i, p := range ordered {
		if versions[i], err = src.Next(ctx, p.Name); err != nil {
			return nil, err
		}
		if versions[i].Releasing() {
			releasing[p.Name] = versions[i].Version
		}
	}
	if err := s.expect.check(releasing, s.version != ""); err != nil {
		return nil, err
	}

	outcomes := make([]shipOutcome, len(ordered))
	var items []shipItem
	for i, p := range ordered {
		v := versions[i]
		outcomes[i] = shipOutcome{Project: p.Name, Branch: s.branch, Status: shipNoRelease, LastVersion: v.LastVersion}
		if !v.Releasing() {
			continue
		}
		o := &outcomes[i]
		o.Status, o.Version, o.Channel, o.Tag = shipNotStarted, v.Version, v.Channel, v.Tag
		if dir := artifact.Dir(s.top, p.Name, v.Version); !s.dryRun && !s.force {
			if _, err := os.Lstat(dir); err == nil {
				return nil, refuse("project %s %s: the artifact %s already exists: pass --force to replace it (nothing was run)", p.Name, v.Version, dir)
			}
		}
		pctx := shipplan.Context{Version: v.Version, Channel: v.Channel, Tag: v.Tag, GitHead: v.BaseCommit, DryRun: s.dryRun, ArtifactsDir: s.ws.ArtifactsDir()}
		plan, err := shipplan.Build(s.ws.Dir(), p, pctx)
		if err != nil {
			return nil, refuse("project %s: %v", p.Name, err)
		}
		items = append(items, shipItem{version: v, project: p, plan: plan, outcome: o})
	}
	s.listShip(ordered, versions, items)
	if len(items) == 0 {
		return outcomes, nil
	}
	if err := s.confirm(len(items), "Package", errShipDeclined, "to package without asking (or --dry-run to rehearse)"); err != nil {
		return nil, err
	}
	steps := s.opts.ShipSteps
	if steps == nil {
		steps = func(root string, out io.Writer) shipplan.StepRunner {
			return &shiprun.Runner{RepoRoot: root, Output: out}
		}
	}
	s.exec = shipplan.Executor{Steps: steps(s.ws.Dir(), s.stderr), Output: s.stderr}
	for _, it := range items {
		fmt.Fprintf(s.stderr, "nerv ship: %s %s\n", it.project.Name, it.version.Version)
		if err := s.shipOne(ctx, it); err != nil {
			it.outcome.Status = shipFailed
			it.outcome.Error = err.Error()
			fmt.Fprintf(s.stderr, "nerv ship: %s failed: %v\n%s\n", it.project.Name, err, it.outcome.Action)
			return outcomes, fmt.Errorf("project %s failed (see above)", it.project.Name)
		}
	}
	return outcomes, nil
}

// versionSource is --version's explicit source, or the native computation
// in the release clone.
func (s *shipRun) versionSource() shipplan.VersionSource {
	if s.version != "" {
		return &explicitVersion{s: s}
	}
	return &computedVersions{s: s}
}

// computedVersions computes every project's next version once with the
// native engine in the release clone (every project, so depends_on applies
// with --project too).
type computedVersions struct {
	s       *shipRun
	results map[string]versioning.Result
}

func (v *computedVersions) Next(ctx context.Context, project string) (shipplan.Version, error) {
	if v.results == nil {
		results, err := v.s.ws.Compute(ctx, nil)
		if err != nil {
			return shipplan.Version{}, err
		}
		v.results = map[string]versioning.Result{}
		for _, r := range results {
			v.results[r.Project] = r
		}
		v.s.configSource = configRepository
	}
	r, ok := v.results[project]
	if !ok {
		return shipplan.Version{}, fmt.Errorf("the version engine computed no result for project %s", project)
	}
	out := shipplan.Version{LastTag: r.LastReleaseGitTag, LastVersion: r.LastReleaseVersion}
	if !r.NewReleasePublished {
		return out, nil
	}
	out.Version, out.Channel, out.Tag = r.NewReleaseVersion, r.NewReleaseChannel, r.NewReleaseGitTag
	out.Notes, out.ChangelogSection = r.NewReleaseNotes, r.ChangelogSection
	out.BaseCommit = r.NewReleaseGitHead
	if out.BaseCommit == "" {
		out.BaseCommit = v.s.ws.OriginHead()
	}
	return out, nil
}

// explicitVersion is --version: the version is not computed. The tag
// follows the project's tag_format, the channel is the branch's (from the
// ship: section's branches, as the engine gives it), and the version must
// fit the branch (a prerelease with the branch's prerelease identifier on a
// prerelease branch, a stable version on a release branch) and be newer
// than the project's last release on the branch, as the engine picks it
// (the tags reachable from the branch, of its channel: a stable tag on
// another branch does not count). The engine writes its notes and
// CHANGELOG section from the commits since that release.
type explicitVersion struct {
	s      *shipRun
	branch *versioning.ReleaseBranch
}

func (v *explicitVersion) Next(ctx context.Context, project string) (shipplan.Version, error) {
	s := v.s
	if _, ok := s.ws.Config().Project(project); !ok {
		return shipplan.Version{}, fmt.Errorf("project %q is not in the ship: section", project)
	}
	if v.branch == nil {
		b, err := s.ws.Branch(ctx)
		if err != nil {
			return shipplan.Version{}, refuse("--version %s: %v", s.version, err)
		}
		v.branch = &b
	}
	id, _, _ := strings.Cut(shipplan.Prerelease(s.version), ".")
	switch b := v.branch; {
	case b.Prerelease == "" && id != "":
		return shipplan.Version{}, refuse("--version %s: %s is a release branch, which releases stable versions; pass one without a prerelease, such as %s",
			s.version, b.Name, strings.SplitN(s.version, "-", 2)[0])
	case b.Prerelease != "" && id != b.Prerelease:
		return shipplan.Version{}, refuse("--version %s: %s is a prerelease branch whose prerelease identifier is %s; pass a version such as %s-%s.1",
			s.version, b.Name, b.Prerelease, strings.SplitN(s.version, "-", 2)[0], b.Prerelease)
	}
	r, err := s.ws.Explicit(ctx, project, s.version)
	if err != nil {
		return shipplan.Version{}, refuse("--version %s: %v", s.version, err)
	}
	if last := r.LastReleaseVersion; last != "" {
		if c, _ := shipplan.CompareVersions(s.version, last); c <= 0 {
			return shipplan.Version{}, refuse("--version %s: project %s already has %s (tag %s); the version must be newer than the last one", s.version, project, last, r.LastReleaseGitTag)
		}
	}
	if !r.NewReleasePublished {
		return shipplan.Version{}, refuse("--version %s: %s", s.version, r.Reason)
	}
	return shipplan.Version{
		Version: s.version, Channel: v.branch.Channel, Tag: r.NewReleaseGitTag,
		Notes: r.NewReleaseNotes, ChangelogSection: r.ChangelogSection,
		LastTag: r.LastReleaseGitTag, LastVersion: r.LastReleaseVersion, BaseCommit: s.ws.OriginHead(),
	}, nil
}

// listShip prints what will be packaged, with each plan, on stderr.
func (s *shipRun) listShip(ordered []shipconfig.Project, versions []shipplan.Version, items []shipItem) {
	what := fmt.Sprintf("%d project(s) to package", len(items))
	if s.dryRun {
		what += " (dry run: the pipeline runs, no artifact is written)"
	}
	fmt.Fprintf(s.stderr, "nerv ship: %s from origin/%s (%s)\n", what, s.branch, short(s.ws.OriginHead()))
	switch {
	case s.version != "":
		fmt.Fprintf(s.stderr, "nerv ship: version %s from --version (not computed)\n", s.version)
	case s.configSource != "":
		fmt.Fprintf(s.stderr, "nerv ship: version config: %s\n", configSourceText(s.configSource, ".ai-workflow.yml"))
	}
	for i, p := range ordered {
		if !versions[i].Releasing() {
			fmt.Fprintf(s.stderr, "%s: no new release (last %s)\n", p.Name, orNone(versions[i].LastVersion))
		}
	}
	for _, it := range items {
		fmt.Fprint(s.stderr, it.plan.Render())
		if v := it.version; v.LastVersion != "" {
			fmt.Fprintf(s.stderr, "  last release %s (tag %s)\n", v.LastVersion, v.LastTag)
		} else {
			fmt.Fprintln(s.stderr, "  last release: none")
		}
		if !s.dryRun {
			fmt.Fprintf(s.stderr, "  artifact %s\n", artifact.Dir(s.top, it.project.Name, it.version.Version))
		}
	}
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// confirm asks before a real run on an interactive terminal ("<verb> N
// project(s)? [y/N]"); --yes skips it, and without a terminal --yes is
// required (the refusal says what --yes does). A dry run never asks; "no"
// is declined.
func (s *shipRun) confirm(n int, verb string, declined error, yesDoes string) error {
	if s.dryRun || s.yes {
		return nil
	}
	if s.opts.IsTerminal == nil || !s.opts.IsTerminal() || s.opts.Stdin == nil {
		return refuse("confirmation needed but stdin is not an interactive terminal: pass --yes %s", yesDoes)
	}
	fmt.Fprintf(s.stderr, "%s %d project(s)? [y/N]: ", verb, n)
	line, sawNewline := readAnswer(s.opts.Stdin)
	if !sawNewline {
		fmt.Fprintln(s.stderr)
	}
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		return &refusal.Error{Err: declined}
	}
	return nil
}

// shipOne builds one project: the release files in the clone, the
// pipeline, the prepare hooks, then the artifact. A dry run stops after
// the prepare hooks. The outcome records what happened and, on failure,
// what the user must do; a failure runs the fail hooks.
func (s *shipRun) shipOne(ctx context.Context, it shipItem) error {
	v, o, name := it.version, it.outcome, it.project.Name
	notes := filepath.Join(s.ws.NotesDir(), name+".md")
	if err := os.WriteFile(notes, []byte(v.Notes), 0o600); err != nil {
		return err
	}
	vars := shiprun.Vars{
		Project: name, ProjectPath: it.project.Path, Version: v.Version, Channel: v.Channel,
		Tag: v.Tag, GitHead: v.BaseCommit, LastVersion: v.LastVersion, NotesFile: notes,
		Branch: s.branch, DryRun: s.dryRun,
	}
	failed := func(err error, hooks bool) error {
		o.Action = "Nothing was written to your repository or to origin for " + name + ". Fix the cause. " + s.rerunAdvice(ctx)
		if hooks {
			s.failHooks(ctx, it, vars)
		}
		return err
	}
	files, err := s.writeReleaseFiles(it)
	if err != nil {
		return failed(err, false)
	}
	if err := s.exec.Run(ctx, it.plan, vars); err != nil {
		return failed(err, true)
	}
	if err := s.exec.RunHooks(ctx, it.plan, "prepare", vars); err != nil {
		return failed(err, true)
	}
	if s.dryRun {
		o.Status = shipDryRun
		return nil
	}
	outputs, err := it.plan.Outputs()
	if err != nil {
		return failed(err, true)
	}
	m := artifact.Manifest{
		Project: name, Version: v.Version, Channel: v.Channel, Tag: v.Tag, BaseCommit: v.BaseCommit,
		Branch: s.branch, BuildMode: it.plan.Mode, CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	for _, img := range it.plan.Images {
		m.Images = append(m.Images, img.Refs...)
	}
	dir, err := artifact.Write(s.top, artifact.Input{Manifest: m, Notes: v.Notes, ChangelogSection: v.ChangelogSection, Outputs: outputs, ReleaseFiles: files}, s.force)
	if err != nil {
		return failed(err, true)
	}
	o.Status, o.Artifact = shipPackaged, dir
	fmt.Fprintf(s.stderr, "nerv ship: %s %s: artifact %s\n", name, v.Version, dir)
	return nil
}

// writeReleaseFiles writes, in the release clone only, the version into
// each of build.version_files (with the release link as the package notes
// when origin has a public address) and the CHANGELOG section into the
// project's changelog, and returns their new contents for the artifact.
func (s *shipRun) writeReleaseFiles(it shipItem) ([]artifact.ReleaseFileSource, error) {
	v, p := it.version, it.project
	var out []artifact.ReleaseFileSource
	read := func(rel string) error {
		data, err := os.ReadFile(filepath.Join(s.ws.Dir(), filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		out = append(out, artifact.ReleaseFileSource{Path: rel, Content: data})
		return nil
	}
	var files []string
	if p.Build != nil {
		files = p.Build.VersionFiles
	}
	link := shipclone.ReleaseURL(s.ws.PublicURL(), v.Tag)
	for _, f := range files {
		path := filepath.Join(s.ws.Dir(), filepath.FromSlash(f))
		var err error
		if link == "" {
			fmt.Fprintf(s.stderr, "nerv ship: warning: origin has no public address to link the release to; %s keeps its <PackageReleaseNotes>\n", f)
			err = shipplan.WriteCsprojVersion(path, v.Version)
		} else {
			err = shipplan.WriteCsproj(path, v.Version, link)
		}
		if err != nil {
			return nil, err
		}
		if err := read(f); err != nil {
			return nil, err
		}
	}
	if p.Changelog != "" && strings.TrimSpace(v.ChangelogSection) != "" {
		path := filepath.Join(s.ws.Dir(), filepath.FromSlash(p.Changelog))
		old, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(prependChangelog(string(old), v.ChangelogSection)), 0o644); err != nil {
			return nil, err
		}
		if err := read(p.Changelog); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// prependChangelog puts section at the top of a CHANGELOG, under its
// "# " title when it has one.
func prependChangelog(old, section string) string {
	section = strings.TrimRight(section, "\r\n") + "\n"
	switch {
	case strings.TrimSpace(old) == "":
		return section
	case strings.HasPrefix(old, "# "):
		title, rest, _ := strings.Cut(old, "\n")
		return title + "\n\n" + section + "\n" + strings.TrimLeft(rest, "\r\n")
	}
	return section + "\n" + old
}

// rerunAdvice says how to run nerv ship again: after a pull when origin's
// branch moved during this run, since nerv ship refuses a local branch
// behind origin.
//
// Origin is read under its own short deadline, detached from ctx, so an
// interrupted run still gets accurate advice.
func (s *shipRun) rerunAdvice(ctx context.Context) string {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rerunCheckTimeout)
	defer cancel()
	head, err := s.ws.BranchOnOrigin(rctx)
	switch {
	case err != nil:
		return fmt.Sprintf("Check whether origin's %s moved during this run (git fetch); if it did, pull first (git pull), then run nerv ship again.", s.branch)
	case head == "":
		return fmt.Sprintf("Branch %s was deleted on origin during this run: check what happened before releasing it again.", s.branch)
	case head != s.ws.OriginHead():
		return fmt.Sprintf("Origin's %s moved during this run (a release commit was pushed): pull first (git pull), then run nerv ship again.", s.branch)
	}
	return "Then run nerv ship again."
}

// rerunCheckTimeout bounds the read of origin's branch for the rerun advice.
const rerunCheckTimeout = 10 * time.Second

// failHooks runs the project's fail hooks, unless the run was interrupted
// (a second Ctrl-C must not be needed to stop nerv ship).
func (s *shipRun) failHooks(ctx context.Context, it shipItem, vars shiprun.Vars) {
	if it.plan == nil || len(it.plan.Hooks["fail"]) == 0 {
		return
	}
	if ctx.Err() != nil {
		fmt.Fprintf(s.stderr, "nerv ship: %s: fail hooks not run: interrupted\n", vars.Project)
		return
	}
	if err := s.exec.RunHooks(ctx, it.plan, "fail", vars); err != nil {
		fmt.Fprintf(s.stderr, "nerv ship: %s: a fail hook failed too: %v\n", vars.Project, err)
	}
}

func listOrNothing(names []string) string {
	if len(names) == 0 {
		return "nothing"
	}
	return strings.Join(names, ", ")
}

// writeShipSummary writes the final summary in one write: the JSON
// document, or one line per project with the action after a failure.
func writeShipSummary(w io.Writer, asJSON, dryRun bool, doc shipDocument) error {
	outcomes := doc.Projects
	var b bytes.Buffer
	if asJSON {
		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
	} else {
		writeShipText(&b, dryRun, outcomes)
	}
	if _, err := w.Write(b.Bytes()); err != nil {
		return err
	}
	if f, ok := w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

func writeShipText(b *bytes.Buffer, dryRun bool, outcomes []shipOutcome) {
	releasing := slices.ContainsFunc(outcomes, func(o shipOutcome) bool { return o.Status != shipNoRelease })
	if !releasing {
		b.WriteString("nothing to release\n")
	}
	for _, o := range outcomes {
		if o.Status == shipNoRelease {
			fmt.Fprintf(b, "%s: no new release (last %s)\n", o.Project, orNone(o.LastVersion))
			continue
		}
		fmt.Fprintf(b, "%s %s (tag %s): ", o.Project, o.Version, o.Tag)
		switch o.Status {
		case shipDryRun:
			b.WriteString("dry run: pipeline and prepare hooks passed; nothing packaged\n")
		case shipPackaged:
			fmt.Fprintf(b, "packaged in %s\n", o.Artifact)
		case shipNotStarted:
			b.WriteString("not started\n")
		default:
			fmt.Fprintf(b, "failed\n  %s\n  %s\n", o.Error, o.Action)
		}
	}
	if dryRun && releasing {
		b.WriteString("dry run: no artifact was written\n")
	}
}
