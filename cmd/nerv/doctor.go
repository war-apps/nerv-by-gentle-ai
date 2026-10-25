package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configstore"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/doctor"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/skills"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

const doctorUsage = `Usage: nerv doctor [--json] [--cwd <dir>]

Check that everything NERV needs is in order, on this machine (gentle-ai,
Claude Code, engram, the NERV plugin and its skills, the user
configuration) and in the repository (its configuration, a pending
nerv migrate, the origin remote and base branch, the project extensions
and providers, and the release:, deploy: and tasks: sections with the
extensions they need). It changes nothing: it lists what it found, then
the steps to take and the optional ones.

Flags:
  --json         Print the results as JSON
  --cwd <dir>    A directory inside the repository (default ".")

Exit codes: 0 nothing failed (there may be warnings); 1 a check failed;
2 usage or environment error.
`

const stepMigrate = "Run nerv migrate --dry-run to see the steps, then nerv migrate"

func runDoctor(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cwd := fs.String("cwd", ".", "")
	jsonOutput := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, doctorUsage)
			return 0
		}
		fmt.Fprintf(stderr, "nerv doctor: %v\n\n%s", err, doctorUsage)
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "nerv doctor: unexpected argument %q\n\n%s", fs.Arg(0), doctorUsage)
		return 2
	}
	home, err := configure.ResolveHome(opts.Home)
	if err != nil {
		fmt.Fprintf(stderr, "nerv doctor: %v\n", err)
		return 2
	}
	d := doctorRun{ctx: context.Background(), opts: opts, home: home, paths: paths.Resolve(home)}
	d.machine()
	d.repository(*cwd)
	if *jsonOutput {
		data, _ := json.MarshalIndent(struct {
			Health string `json:"health"`
			doctor.Report
		}{d.report.Health(), d.report}, "", "  ")
		fmt.Fprintln(stdout, string(data))
	} else {
		fmt.Fprint(stdout, doctor.Render(d.report))
	}
	if d.report.Count(doctor.Fail) > 0 {
		return 1
	}
	return 0
}

type doctorRun struct {
	ctx    context.Context
	opts   options
	home   string
	paths  paths.Paths
	report doctor.Report
	// user is the user configuration, nil when it could not be read.
	user *config.Document
}

func (d *doctorRun) add(name string, status doctor.Status, detail, step string) {
	d.report.Add(name, status, detail, step)
}

// machine checks the tools, the NERV plugin and its skills, and the user
// configuration.
func (d *doctorRun) machine() {
	d.report.Section("This machine")
	pre := gentleai.CheckPreflight(d.ctx, d.opts.Runner)
	switch {
	case !pre.Found:
		d.add("gentle-ai", doctor.Fail, "not found on PATH", "Install gentle-ai 4.x (https://github.com/Gentleman-Programming/gentle-ai)")
	case pre.Version == "":
		d.add("gentle-ai", doctor.Warn, "found, but gentle-ai --version printed no version", "Check that gentle-ai --version works")
	case !pre.OK:
		d.add("gentle-ai", doctor.Fail, pre.Version+": NERV needs gentle-ai 4.x", "Install gentle-ai 4.x (https://github.com/Gentleman-Programming/gentle-ai)")
	default:
		d.add("gentle-ai", doctor.OK, pre.Version, "")
	}
	if _, err := d.opts.LookPath("claude"); err != nil {
		d.add("claude", doctor.Fail, "Claude Code (claude) not found on PATH", "Install Claude Code (https://claude.com/claude-code)")
	} else {
		d.add("claude", doctor.OK, "found", "")
	}
	if _, err := d.opts.LookPath("engram"); err != nil {
		d.add("engram", doctor.Warn, "not found on PATH: the agents run without persistent memory", "Install engram (gentle-ai installs it)")
	} else {
		d.add("engram", doctor.OK, "found", "")
	}
	d.plugin()
	d.skills()
	d.userConfig()
}

func (d *doctorRun) settings() *claude.Settings {
	path := d.opts.Settings
	if path == "" {
		path = d.paths.Settings
	}
	s, err := claude.LoadSettings(path)
	if err != nil {
		return claude.NewSettings()
	}
	return s
}

// plugin checks that Claude Code has the NERV plugin this binary ships,
// enabled.
func (d *doctorRun) plugin() {
	const step = "Run nerv install"
	want, err := version.PluginVersion(d.opts.PluginFS)
	if err != nil {
		d.add("plugin", doctor.Fail, err.Error(), "")
		return
	}
	info, found, err := claude.InstalledPlugin(d.paths.InstalledPlugins, install.PluginID)
	switch {
	case err != nil:
		d.add("plugin", doctor.Fail, err.Error(), step)
	case !found:
		d.add("plugin", doctor.Fail, install.PluginID+" is not installed in Claude Code", step)
	case !d.settings().PluginEnabled(install.PluginID):
		d.add("plugin", doctor.Fail, install.PluginID+" is installed but disabled in Claude Code", step)
	case info.Version != want:
		d.add("plugin", doctor.Warn, fmt.Sprintf("Claude Code has %s %s; this nerv ships %s", install.PluginID, info.Version, want), step+" to update it")
	default:
		d.add("plugin", doctor.OK, install.PluginID+" "+info.Version+", enabled", "")
	}
}

// skills checks that the skills the NERV agents load are installed.
func (d *doctorRun) skills() {
	manifest, err := skills.LoadManifestFS(d.opts.PluginFS)
	if err != nil {
		d.add("skills", doctor.Fail, err.Error(), "")
		return
	}
	var missing []string
	statuses := skills.Status(manifest, d.paths.SkillsDir)
	for _, s := range statuses {
		if !s.Installed {
			missing = append(missing, s.Name)
		}
	}
	if len(missing) > 0 {
		d.add("skills", doctor.Warn, fmt.Sprintf("%d missing: %s", len(missing), strings.Join(missing, ", ")), "Run nerv skills to install the missing skills")
		return
	}
	d.add("skills", doctor.OK, fmt.Sprintf("all %d installed", len(statuses)), "")
}

// userConfig checks the user configuration: an old one nerv migrate
// --user moves, or one that does not parse.
func (d *doctorRun) userConfig() {
	const name = "user config"
	const step = "Run nerv migrate --user --dry-run to see the steps, then nerv migrate --user"
	if err := d.paths.RefusePreviousUser(); err != nil {
		d.add(name, doctor.Fail, d.paths.PreviousUserConfig+" is from an earlier nerv: no nerv command reads it", step)
		return
	}
	doc, exists, err := (configstore.Store{}).Load(d.paths.UserConfig)
	switch {
	case err != nil:
		d.add(name, doctor.Fail, err.Error(), "Fix "+d.paths.UserConfig)
	case !exists && isFile(d.paths.LegacyUserConfig):
		d.add(name, doctor.Warn, d.paths.LegacyUserConfig+" is the nerv 4.2.0 configuration", step)
	case !exists:
		d.user = doc
		d.add(name, doctor.OK, "none: the defaults apply", "")
	default:
		d.user = doc
		runTime, err := workflowconfig.RunTimeUserKeys(doc, d.personal())
		switch {
		case err != nil:
			d.add(name, doctor.Fail, d.paths.UserConfig+": "+err.Error(), "Fix "+d.paths.UserConfig)
		case len(runTime) > 0:
			d.add(name, doctor.Warn, strings.Join(runTime, ", ")+" no longer apply: tasks, release and deploy are read from each project's .ai-workflow/config.yaml", step)
		default:
			d.add(name, doctor.OK, d.paths.UserConfig, "")
		}
	}
}

// personal is the keys of the tasks:, release: and deploy: sections that
// stay in the user file: the personal task keys and the scope: user keys
// of the providers nerv ships.
func (d *doctorRun) personal() workflowconfig.Personal {
	fsys := d.opts.ExtensionsFS
	if fsys == nil {
		fsys = nerv.ExtensionsFS()
	}
	p, err := workflowconfig.LoadPersonal(fsys)
	if err != nil {
		return workflowconfig.Personal{}
	}
	return p
}

// repository checks the repository the directory cwd is in, if any.
func (d *doctorRun) repository(cwd string) {
	d.report.Section("This repository")
	top, err := gitTopLevel(d.ctx, d.opts.Runner, cwd)
	if err == nil && top == "" {
		err = &refusal.Error{Err: fmt.Errorf("%s is not inside a git repository", cwd)}
	}
	if err != nil {
		if refusal.Is(err) {
			d.add("repository", doctor.Info, err.Error()+": the repository checks were skipped", "")
		} else {
			d.add("repository", doctor.Fail, err.Error(), "Install git")
		}
		return
	}
	d.report.Section("This repository (" + top + ")")
	project, ok := d.projectConfig(top)
	origin := originURL(d.ctx, d.opts, top)
	d.migration(top, origin)
	if !ok {
		return
	}
	wc := workflowconfig.Config{Project: project, User: d.user}
	scalar := func(key string) string {
		v, _ := wc.Scalar(key)
		return v
	}
	d.projectExtensions(top)
	release, deploy := d.release(top), d.deploy(top)
	d.tasks(scalar("tasks.provider"))
	switch {
	case origin != "":
		d.add("origin", doctor.OK, origin, "")
	case release || deploy:
		d.add("origin", doctor.Warn, "no origin remote: nerv release and nerv deploy work from origin", "Add the origin remote: git remote add origin <url>")
	default:
		d.add("origin", doctor.Info, "no origin remote", "")
	}
	d.baseBranch(top, scalar("git.base_branch"))
}

// projectConfig checks .ai-workflow/config.yaml, and returns it and
// whether the checks that read it can run.
func (d *doctorRun) projectConfig(top string) (*config.Document, bool) {
	const name = "config"
	if err := paths.RefusePreviousProject(top); err != nil {
		d.add(name, doctor.Fail, paths.PreviousProjectConfigFile+" is from an earlier nerv: no nerv command reads it", stepMigrate)
		return nil, false
	}
	path := paths.ProjectConfig(top)
	doc, exists, err := (configstore.Store{}).Load(path)
	switch {
	case err != nil:
		d.add(name, doctor.Fail, err.Error(), "Fix "+path)
		return nil, false
	case !exists && isFile(paths.LegacyProjectConfig(top)):
		d.add(name, doctor.Warn, ".nerv/nerv.yaml is the nerv 4.2.0 configuration", stepMigrate)
		return nil, false
	case !exists:
		d.add(name, doctor.Warn, "NERV is not set up here: there is no .ai-workflow/config.yaml", "Run /nerv:init in Claude Code to set NERV up in this repository")
		return nil, false
	}
	if v, _ := doc.Scalar("enabled"); v != "true" {
		d.add(name, doctor.Warn, "NERV is off here: .ai-workflow/config.yaml does not say enabled: true", "Set enabled: true in .ai-workflow/config.yaml to turn NERV on")
	} else {
		d.add(name, doctor.OK, ".ai-workflow/config.yaml, enabled", "")
	}
	return doc, true
}

// migration checks whether nerv migrate has steps to take in the
// repository.
func (d *doctorRun) migration(top, origin string) {
	plan, err := migrate.Compute(migrate.Options{RepoRoot: top, Home: d.home, ExtensionsFS: d.opts.ExtensionsFS, OriginURL: origin, Now: d.opts.Now()})
	switch {
	case err != nil:
		d.add("migration", doctor.Fail, "nerv migrate cannot run: "+err.Error(), "Fix what nerv migrate reports, then run it")
	case !plan.Empty():
		d.add("migration", doctor.Warn, fmt.Sprintf("nerv migrate has %d step(s) to take", len(plan.Changes)), stepMigrate)
	default:
		d.add("migration", doctor.OK, "nothing to migrate", "")
	}
}

// projectExtensions checks the extensions and providers under
// .ai-workflow/, when there are any.
func (d *doctorRun) projectExtensions(top string) {
	if !isDir(paths.ProjectExtensionsDir(top)) && !isDir(paths.ProjectProvidersDir(top)) {
		return
	}
	fsys := d.opts.ExtensionsFS
	if fsys == nil {
		fsys = nerv.ExtensionsFS()
	}
	merged, err := extension.WithProject(fsys, top)
	if err != nil {
		d.add("project extensions", doctor.Fail, err.Error(), "Fix the folder it names under .ai-workflow/")
		return
	}
	d.add("project extensions", doctor.OK, fmt.Sprintf("%d valid", len(extension.ProjectFolders(merged))), "")
}

// release checks the release: section and that the release extension is
// installed, and reports whether the section is there.
func (d *doctorRun) release(top string) bool {
	rel, err := releaseconfig.Load(top)
	if errors.Is(err, releaseconfig.ErrNotConfigured) {
		d.add("release", doctor.Info, "not configured", "Run /release:setup in Claude Code to build and package releases with nerv release")
		return false
	}
	if err == nil {
		err = rel.CheckBuild()
	}
	if err != nil {
		d.add("release", doctor.Fail, err.Error(), "Fix the release: section of .ai-workflow/config.yaml (/release:setup helps)")
		return true
	}
	if !d.extensionEnabled("release") {
		d.add("release", doctor.Warn, "configured, but the release extension is not installed", "Run nerv install --with release")
		return true
	}
	d.add("release", doctor.OK, fmt.Sprintf("%d project(s)", len(rel.Projects)), "")
	return true
}

// deploy checks the deploy: section and that the deploy extension is
// installed, and reports whether the section is there.
func (d *doctorRun) deploy(top string) bool {
	dep, err := deployconfig.Load(top)
	switch {
	case errors.Is(err, deployconfig.ErrNotConfigured):
		d.add("deploy", doctor.Info, "not configured", "Run /deploy:setup in Claude Code to deploy releases with nerv deploy")
		return false
	case err != nil:
		d.add("deploy", doctor.Fail, err.Error(), "Fix the deploy: section of .ai-workflow/config.yaml (/deploy:setup helps)")
	case !d.extensionEnabled("deploy"):
		d.add("deploy", doctor.Warn, "configured, but the deploy extension is not installed", "Run nerv install --with deploy")
	default:
		d.add("deploy", doctor.OK, fmt.Sprintf("%d target(s)", len(dep.Targets)), "")
	}
	return true
}

// tasks checks that the task extension and the provider tasks.provider
// names are installed.
func (d *doctorRun) tasks(provider string) {
	if provider == "" {
		d.add("tasks", doctor.Info, "no task provider (tasks.provider)", "")
		return
	}
	var missing []string
	for _, name := range []string{"task", "task-" + provider} {
		if !d.extensionEnabled(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		d.add("tasks", doctor.Warn, "tasks.provider is "+provider+", but "+strings.Join(missing, " and ")+" is not installed", "Run nerv install --with "+strings.Join(missing, ","))
		return
	}
	d.add("tasks", doctor.OK, provider, "")
}

// extensionEnabled reports whether Claude Code has the extension or
// provider plugin <name>@nerv enabled.
func (d *doctorRun) extensionEnabled(name string) bool {
	return d.settings().PluginEnabled(name + "@" + install.MarketplaceName)
}

// baseBranch checks that git.base_branch exists, locally or on origin.
func (d *doctorRun) baseBranch(top, branch string) {
	if branch == "" {
		branch = configDefault("git.base_branch")
	}
	has := func(ref string) bool {
		out, _, code, err := d.opts.Runner.Run(d.ctx, "git", "-C", top, "rev-parse", "--verify", "--quiet", ref)
		return err == nil && code == 0 && strings.TrimSpace(out) != ""
	}
	switch {
	case has("refs/heads/" + branch):
		d.add("base branch", doctor.OK, branch, "")
	case has("refs/remotes/origin/" + branch):
		d.add("base branch", doctor.OK, branch+" (on origin only)", "")
	default:
		d.add("base branch", doctor.Warn, branch+" does not exist", "Create the "+branch+" branch, or set git.base_branch in .ai-workflow/config.yaml")
	}
}

func configDefault(key string) string {
	for _, kd := range config.Defaults() {
		if kd.Key == key {
			return kd.Value
		}
	}
	return ""
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
