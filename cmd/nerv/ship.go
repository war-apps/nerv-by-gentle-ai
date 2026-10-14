package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipplan"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

const shipUsage = `Usage: nerv ship [flags]

Releases the repository (or each project of the release: section of
.nerv/nerv.yaml) from your machine, as the CI release would: in a fresh
temporary clone of origin's branch, for each project with a new release in
dependency order, it writes the dotnet project file, runs the pipeline
(restore, lint, build, test, coverage, package) and the prepare hooks, runs
semantic-release for real with the repository's own configuration (release
commit, CHANGELOG, tag, GitHub release), then the publish stage (docker,
nuget, npm) and the publish and success hooks. Your repository is never
modified, and only what origin has is released: a local branch ahead of or
behind origin is refused.

Flags:
  --cwd <dir>              A directory inside the repository (default ".")
  --branch <name>          Branch to release (default: the current one)
  --project <name>         Only this project of the release: section
  --dry-run                Run the pipeline and the prepare hooks in the
                            clone, but never semantic-release or publish
  --yes                    Release without asking for confirmation
  --fetch-timeout <dur>    Bound for each read of origin (default 60s)
  --json                   Print the final summary as JSON

Credentials stay with git (ssh keys, credential helpers), docker and the
environment (NUGET_API_KEY, NPM_TOKEN); GH_TOKEN, when not set, is read
with "gh auth token" and given to semantic-release through its environment
only. Progress goes to stderr, the final summary to stdout. Exit codes: 0
released (also when there is nothing to release, and a dry run), 1 refused
(invalid configuration, unknown project, local branch not equal to origin's,
no confirmation), 2 a step or the environment failed, 128 + the signal
when interrupted.
`

// shipWorkspace is the release clone nerv ship works in; *semrel.Workspace
// is the real one.
type shipWorkspace interface {
	Dir() string
	ArtifactsDir() string
	NotesDir() string
	OriginHead() string
	PublicURL() string
	Compute(ctx context.Context, project string) ([]semrel.ProjectResult, error)
	CheckPush(ctx context.Context) error
	Release(ctx context.Context, req semrel.ReleaseRequest) (semrel.ProjectResult, error)
	TagOnOrigin(ctx context.Context, tag string) (bool, error)
	Close() error
}

// Statuses of a project in the final summary.
const (
	shipReleased   = "released"
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
	// Tagged reports that the release tag is on origin.
	Tagged bool `json:"tagged"`
	// Published lists the publish-stage steps that completed (preset steps
	// such as docker:push <image>:<tag> or nuget:push, a publish override,
	// before_/after_publish hooks).
	Published []string `json:"published"`
	Error     string   `json:"error,omitempty"`
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

	ws shipWorkspace
	// computed are every computed result, the dependencies of --project
	// included: Release needs them for depends_on. results are the
	// projects to ship (only --project's when given).
	computed []semrel.ProjectResult
	results  []semrel.ProjectResult
	exec     shipplan.Executor
	tokenEnv []string
}

func runShip(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("ship", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	s := &shipRun{opts: opts, stdout: stdout, stderr: stderr}
	fs.StringVar(&s.cwd, "cwd", ".", "")
	fs.StringVar(&s.branch, "branch", "", "")
	fs.StringVar(&s.project, "project", "", "")
	fs.BoolVar(&s.dryRun, "dry-run", false, "")
	fs.BoolVar(&s.yes, "yes", false, "")
	fs.DurationVar(&s.timeout, "fetch-timeout", semrel.DefaultFetchTimeout, "")
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

	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop, received := signalCtx(context.Background())
	defer stop()

	outcomes, err := s.execute(ctx)
	if outcomes != nil {
		if werr := writeShipSummary(stdout, *jsonOut, s.dryRun, outcomes); werr != nil && err == nil {
			err = fmt.Errorf("writing the summary: %w", werr)
		}
	}
	if err == nil {
		return 0
	}
	if ctx.Err() != nil {
		if sig := received(); sig != nil {
			fmt.Fprintf(stderr, "nerv ship: interrupted (%v)\n", sig)
			return interruptedExitCode(sig)
		}
		fmt.Fprintln(stderr, "nerv ship: interrupted")
		return 2
	}
	fmt.Fprintf(stderr, "nerv ship: %v\n", err)
	var invalid *shipconfig.ValidationError
	if errors.As(err, &invalid) {
		return 1
	}
	return exitCodeFor(err)
}

func shipUsageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "nerv ship: %s\n\n%s", problem, shipUsage)
	return 2
}

func refuse(format string, args ...any) error {
	return &refusal.Error{Err: fmt.Errorf(format, args...)}
}

// execute runs the release and returns the projects' outcomes (nil when it
// stopped before knowing them).
func (s *shipRun) execute(ctx context.Context) ([]shipOutcome, error) {
	top, err := gitTopLevel(ctx, s.opts.Runner, s.cwd)
	if err != nil {
		return nil, err
	}
	// Refuse an invalid configuration or an unknown --project before
	// reading origin.
	if _, err := requestShape(top, s.project); err != nil {
		return nil, err
	}
	local, err := s.localBranch(ctx, top)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(s.stderr, "nerv ship: reading origin's %s into a release clone of %s\n", s.branch, top)
	open := s.opts.OpenShipWorkspace
	if open == nil {
		open = func(ctx context.Context, o semrel.WorkspaceOptions) (shipWorkspace, error) {
			return semrel.NewWorkspace(ctx, o)
		}
	}
	ws, err := open(ctx, semrel.WorkspaceOptions{RepoRoot: top, Branch: s.branch, FetchTimeout: s.timeout})
	if errors.Is(err, semrel.ErrNoOrigin) {
		return nil, refuse("the repository has no origin remote: nerv ship releases what is on origin")
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
		return nil, refuse("branch %s is not on origin: push it first (nerv ship releases what is on origin)", s.branch)
	case head != local:
		return nil, refuse("your local %s (%s) is not origin/%s (%s): nerv ship releases what is on origin and never pushes your commits; push or pull first",
			s.branch, short(local), s.branch, short(head))
	}

	// Every project is computed, even with --project: Compute returns only
	// the requested project, and Release needs the dependencies' results to
	// apply depends_on.
	results, err := ws.Compute(ctx, "")
	if err != nil {
		return nil, err
	}
	s.computed = results
	if s.project != "" {
		results = slices.DeleteFunc(slices.Clone(results), func(r semrel.ProjectResult) bool { return r.Project != s.project })
	}
	s.results = results
	return s.release(ctx)
}

// localBranch resolves the branch to release (default: the current one) and
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

// shipItem is one releasing project with its configuration and plan.
type shipItem struct {
	result  semrel.ProjectResult
	project shipconfig.Project
	plan    *shipplan.Plan
	outcome *shipOutcome
}

// release plans every releasing project, shows the plans, asks for
// confirmation and ships them in order, stopping at the first failure.
func (s *shipRun) release(ctx context.Context) ([]shipOutcome, error) {
	rel, err := shipconfig.Load(s.ws.Dir())
	if errors.Is(err, shipconfig.ErrNotConfigured) {
		rel, err = nil, nil
	}
	if err != nil {
		return nil, err
	}
	outcomes := make([]shipOutcome, len(s.results))
	var items []shipItem
	for i, r := range s.results {
		outcomes[i] = shipOutcome{Project: r.Project, Branch: r.Branch, Status: shipNoRelease, LastVersion: r.LastReleaseVersion, Published: []string{}}
		if !r.NewReleasePublished {
			continue
		}
		o := &outcomes[i]
		o.Status, o.Version, o.Channel, o.Tag = shipNotStarted, r.NewReleaseVersion, r.NewReleaseChannel, r.NewReleaseGitTag
		p, err := projectConfig(rel, r.Project)
		if err != nil {
			return nil, err
		}
		plan, err := shipplan.Build(s.ws.Dir(), p, shipplan.Context{
			Version: r.NewReleaseVersion, Channel: r.NewReleaseChannel, Tag: r.NewReleaseGitTag, GitHead: r.NewReleaseGitHead,
			DryRun: s.dryRun, ArtifactsDir: s.ws.ArtifactsDir(),
		})
		if err != nil {
			return nil, refuse("project %s: %v", r.Project, err)
		}
		items = append(items, shipItem{result: r, project: p, plan: plan, outcome: o})
	}
	s.list(items)
	if len(items) == 0 {
		return outcomes, nil
	}
	if err := s.confirm(len(items)); err != nil {
		return nil, err
	}
	if !s.dryRun {
		if err := s.ws.CheckPush(ctx); err != nil {
			return nil, err
		}
		s.tokenEnv = s.githubTokenEnv(ctx)
	}
	steps := s.opts.ShipSteps
	if steps == nil {
		steps = func(root string, out io.Writer) shipplan.StepRunner {
			return &shiprun.Runner{RepoRoot: root, Output: out}
		}
	}
	s.exec = shipplan.Executor{Steps: steps(s.ws.Dir(), s.stderr), Output: s.stderr}
	for _, it := range items {
		fmt.Fprintf(s.stderr, "nerv ship: %s %s\n", it.result.Project, it.result.NewReleaseVersion)
		if err := s.ship(ctx, it); err != nil {
			it.outcome.Status = shipFailed
			it.outcome.Error = err.Error()
			fmt.Fprintf(s.stderr, "nerv ship: %s failed: %v\n%s\n", it.result.Project, err, it.outcome.Action)
			return outcomes, fmt.Errorf("project %s failed (see above)", it.result.Project)
		}
	}
	return outcomes, nil
}

// projectConfig is the release: section's project, or, for a repository
// without the section, the whole repository with no presets.
func projectConfig(rel *shipconfig.Release, name string) (shipconfig.Project, error) {
	if rel == nil {
		return shipconfig.Project{Name: name, Path: "."}, nil
	}
	p, ok := rel.Project(name)
	if !ok {
		return shipconfig.Project{}, fmt.Errorf("project %q is not in origin's release: section", name)
	}
	return p, nil
}

// list prints what will be released, with each plan, on stderr.
func (s *shipRun) list(items []shipItem) {
	what := fmt.Sprintf("%d project(s) to release", len(items))
	if s.dryRun {
		what += " (dry run: nothing is released or published)"
	}
	fmt.Fprintf(s.stderr, "nerv ship: %s from origin/%s (%s)\n", what, s.branch, short(s.ws.OriginHead()))
	for _, r := range s.results {
		if !r.NewReleasePublished {
			fmt.Fprintf(s.stderr, "%s: no new release (last %s)\n", r.Project, orNone(r.LastReleaseVersion))
		}
	}
	for _, it := range items {
		r := it.result
		fmt.Fprint(s.stderr, it.plan.Render())
		if r.LastReleaseVersion != "" {
			fmt.Fprintf(s.stderr, "  last release %s (tag %s)\n", r.LastReleaseVersion, r.LastReleaseGitTag)
		} else {
			fmt.Fprintln(s.stderr, "  last release: none")
		}
	}
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// confirm asks before a real release on an interactive terminal; --yes
// skips it, and without a terminal --yes is required. A dry run never asks.
func (s *shipRun) confirm(n int) error {
	if s.dryRun || s.yes {
		return nil
	}
	if s.opts.IsTerminal == nil || !s.opts.IsTerminal() || s.opts.Stdin == nil {
		return refuse("confirmation needed but stdin is not an interactive terminal: pass --yes to release without asking (or --dry-run to rehearse)")
	}
	fmt.Fprintf(s.stderr, "Release %d project(s)? [y/N]: ", n)
	line, sawNewline := readAnswer(s.opts.Stdin)
	if !sawNewline {
		fmt.Fprintln(s.stderr)
	}
	if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
		return refuse("cancelled: nothing was released")
	}
	return nil
}

// githubTokenEnv returns GH_TOKEN for semantic-release when neither GH_TOKEN
// nor GITHUB_TOKEN is set, from `gh auth token`; nil when the environment
// already has one or none can be read. The token only ever travels in the
// runner's environment.
func (s *shipRun) githubTokenEnv(ctx context.Context) []string {
	getenv := s.opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	if getenv("GH_TOKEN") != "" || getenv("GITHUB_TOKEN") != "" {
		return nil
	}
	token := ""
	if s.opts.GitHubToken != nil {
		token = s.opts.GitHubToken(ctx)
	} else if out, _, code, err := s.opts.Runner.Run(ctx, "gh", "auth", "token"); err == nil && code == 0 {
		token = strings.TrimSpace(out)
	}
	if token == "" {
		fmt.Fprintln(s.stderr, "nerv ship: no GH_TOKEN or GITHUB_TOKEN, and `gh auth token` gave none: a GitHub plugin in the semantic-release configuration will fail its checks (before any tag)")
		return nil
	}
	return []string{"GH_TOKEN=" + token}
}

// ship releases one project: dotnet project file, pipeline, prepare hooks,
// semantic-release, publish stage, publish and success hooks. A dry run
// stops after the prepare hooks. The outcome records what happened and,
// on failure, what the user must do.
func (s *shipRun) ship(ctx context.Context, it shipItem) error {
	r, o := it.result, it.outcome
	notes := filepath.Join(s.ws.NotesDir(), r.Project+".md")
	if err := os.WriteFile(notes, []byte(r.NewReleaseNotes), 0o600); err != nil {
		return err
	}
	vars := shiprun.Vars{
		Project: r.Project, ProjectPath: it.project.Path, Version: r.NewReleaseVersion, Channel: r.NewReleaseChannel,
		Tag: r.NewReleaseGitTag, GitHead: r.NewReleaseGitHead, LastVersion: r.LastReleaseVersion, NotesFile: notes,
		Branch: s.branch, DryRun: s.dryRun,
	}
	beforeTag := "Nothing was tagged or published. Fix the cause and run nerv ship again."
	if err := s.writeProjectFile(it); err != nil {
		o.Action = beforeTag
		return err
	}
	if err := s.exec.Run(ctx, it.plan, vars); err != nil {
		o.Action = beforeTag
		return err
	}
	if err := s.exec.RunHooks(ctx, it.plan, "prepare", vars); err != nil {
		o.Action = beforeTag
		return err
	}
	if s.dryRun {
		o.Status = shipDryRun
		return nil
	}

	released, err := s.ws.Release(ctx, semrel.ReleaseRequest{Expected: r, Computed: s.computed, Env: s.tokenEnv, Output: s.stderr})
	if err != nil {
		s.releaseFailed(ctx, it, vars, released, err)
		return err
	}
	o.Tagged = true
	if released.NewReleaseGitHead != "" {
		vars.GitHead = released.NewReleaseGitHead
	}

	pub := it.plan.Publish()
	if err := s.exec.RunPublish(ctx, it.plan, vars); err != nil {
		var f *shipplan.StepFailure
		if errors.As(err, &f) && pub != nil {
			o.Published = append(o.Published, stepNames(pub.Steps[:f.Index])...)
		}
		s.afterTagFailed(ctx, it, vars, publishSteps(it.plan, len(o.Published)))
		return err
	}
	if pub != nil {
		o.Published = append(o.Published, stepNames(pub.Steps)...)
	}
	if err := s.exec.RunHooks(ctx, it.plan, "publish", vars); err != nil {
		s.afterTagFailed(ctx, it, vars, []string{"the publish hooks from the failed one on", "the success hooks"})
		return err
	}
	if err := s.exec.RunHooks(ctx, it.plan, "success", vars); err != nil {
		o.Action = fmt.Sprintf("The release %s is tagged and published; a success hook failed: run what it does by hand if you need it. Do not run nerv ship again for this release: it would compute the next version.", r.NewReleaseGitTag)
		s.failHooks(ctx, it, vars)
		return err
	}
	o.Status = shipReleased
	return nil
}

// writeProjectFile writes the dotnet preset's project file in the clone
// before the pipeline, so the packages carry the version and the release
// link, and the release commit holds the file when the repository's
// @semantic-release/git assets list it. Without a public origin the release
// notes element is left as it is.
func (s *shipRun) writeProjectFile(it shipItem) error {
	p := it.project
	if !slices.Contains(p.Presets, "dotnet") || p.Dotnet == nil || p.Dotnet.ProjectFile == "" {
		return nil
	}
	path := filepath.Join(s.ws.Dir(), filepath.FromSlash(p.Dotnet.ProjectFile))
	link := semrel.ReleaseURL(s.ws.PublicURL(), it.result.NewReleaseGitTag)
	if link == "" {
		fmt.Fprintf(s.stderr, "nerv ship: warning: origin has no public address to link the release to; %s keeps its <PackageReleaseNotes>\n", p.Dotnet.ProjectFile)
		return shipplan.WriteCsprojVersion(path, it.result.NewReleaseVersion)
	}
	return shipplan.WriteCsproj(path, it.result.NewReleaseVersion, link)
}

// releaseFailed records a failed semantic-release run: whether the tag
// reached origin and what to do, then runs the fail hooks.
func (s *shipRun) releaseFailed(ctx context.Context, it shipItem, vars shiprun.Vars, released semrel.ProjectResult, err error) {
	o, tag := it.outcome, it.result.NewReleaseGitTag
	if ctx.Err() != nil {
		o.Action = fmt.Sprintf("Interrupted while semantic-release was running: check whether origin has the tag (git ls-remote --tags origin %s) and the release commit before doing anything else.", tag)
		s.failHooks(ctx, it, vars)
		return
	}
	tags := []string{tag}
	if released.NewReleaseGitTag != "" && released.NewReleaseGitTag != tag {
		tags = append(tags, released.NewReleaseGitTag)
	}
	var checkErr error
	for _, t := range tags {
		ok, err := s.ws.TagOnOrigin(ctx, t)
		if err != nil {
			checkErr = err
		}
		if ok {
			o.Tagged, tag = true, t
		}
	}
	switch {
	case o.Tagged:
		o.Action = fmt.Sprintf("Tag %s is on origin, but semantic-release failed after tagging (its publish or success plugins, e.g. the GitHub release): finish those by hand. NERV's publish stage did not run: publish %s by hand from the tag (git checkout %s). Do not run nerv ship again for this release: it would compute the next version.",
			tag, listOrNothing(publishSteps(it.plan, 0)), tag)
	case checkErr != nil:
		o.Action = fmt.Sprintf("Could not tell whether tag %s reached origin (%v): check with git ls-remote --tags origin %s before running nerv ship again.", tag, checkErr, tag)
	case errors.Is(err, semrel.ErrVersionMismatch):
		o.Action = "semantic-release would have released another version than the one you confirmed (origin changed?): it was stopped before the tag, nothing was tagged or published. Run nerv ship again to review the new plan."
	default:
		o.Action = fmt.Sprintf("No tag %s on origin: nothing was tagged or published (a prepare plugin such as @semantic-release/git may have pushed a release commit: check origin). Fix the cause and run nerv ship again.", tag)
	}
	s.failHooks(ctx, it, vars)
}

// afterTagFailed records a failure once the tag is on origin: what was
// published, what was not, and what to do; then runs the fail hooks.
func (s *shipRun) afterTagFailed(ctx context.Context, it shipItem, vars shiprun.Vars, notRun []string) {
	o, tag := it.outcome, it.result.NewReleaseGitTag
	o.Action = fmt.Sprintf("Tag %s and its release are on origin. Published: %s. Not run: %s. Publish the rest by hand from the tag (git checkout %s); do not run nerv ship again for this release: it would compute the next version.",
		tag, listOrNothing(o.Published), listOrNothing(notRun), tag)
	s.failHooks(ctx, it, vars)
}

// failHooks runs the project's fail hooks, unless the run was interrupted
// (a second Ctrl-C must not be needed to stop nerv ship).
func (s *shipRun) failHooks(ctx context.Context, it shipItem, vars shiprun.Vars) {
	if len(it.plan.Hooks["fail"]) == 0 {
		return
	}
	if ctx.Err() != nil {
		fmt.Fprintf(s.stderr, "nerv ship: %s: fail hooks not run: interrupted\n", it.result.Project)
		return
	}
	if err := s.exec.RunHooks(ctx, it.plan, "fail", vars); err != nil {
		fmt.Fprintf(s.stderr, "nerv ship: %s: a fail hook failed too: %v\n", it.result.Project, err)
	}
}

// publishSteps names the publish stage's steps from index from on, then the
// publish hooks.
func publishSteps(plan *shipplan.Plan, from int) []string {
	var out []string
	if pub := plan.Publish(); pub != nil && from < len(pub.Steps) {
		out = stepNames(pub.Steps[from:])
	}
	return append(out, stepNames(plan.Hooks["publish"])...)
}

func stepNames(steps []shipplan.Step) []string {
	out := make([]string, 0, len(steps))
	for _, st := range steps {
		out = append(out, st.Name)
	}
	return out
}

func listOrNothing(names []string) string {
	if len(names) == 0 {
		return "nothing"
	}
	return strings.Join(names, ", ")
}

// writeShipSummary writes the final summary in one write: JSON (an array of
// outcomes) or one line per project with the action after a failure.
func writeShipSummary(w io.Writer, asJSON, dryRun bool, outcomes []shipOutcome) error {
	var b bytes.Buffer
	if asJSON {
		data, err := json.Marshal(outcomes)
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
			b.WriteString("dry run: pipeline and prepare hooks passed; not released\n")
		case shipReleased:
			b.WriteString("released")
			if len(o.Published) > 0 {
				fmt.Fprintf(b, "; published: %s", strings.Join(o.Published, ", "))
			}
			b.WriteByte('\n')
		case shipNotStarted:
			b.WriteString("not started\n")
		default:
			state := "not tagged"
			if o.Tagged {
				state = "tagged on origin"
			}
			fmt.Fprintf(b, "failed (%s); published: %s\n  %s\n  %s\n", state, listOrNothing(o.Published), o.Error, o.Action)
		}
	}
	if dryRun && releasing {
		b.WriteString("dry run: nothing was released or published\n")
	}
}
