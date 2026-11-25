package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configstore"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/doctor"
	"github.com/war-apps/nerv-by-gentle-ai/internal/ecosystem"
	"github.com/war-apps/nerv-by-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/install"
	"github.com/war-apps/nerv-by-gentle-ai/internal/migrate"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/skills"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

const doctorUsage = `Usage: nerv doctor [--json] [--cwd <dir>]

Check that everything NERV needs is in order, on this machine (gentle-ai,
Claude Code, engram, the NERV plugin and its skills, the user
configuration) and in the repository (its configuration, a pending
nerv migrate, old nerv plugins its .claude settings still enable, the
origin remote and the base branch). It changes
nothing: it lists what it found, then the steps to take and the optional
ones.

Flags:
  --json         Print the results as JSON
  --cwd <dir>    A directory inside the repository (default ".")

Exit codes: 0 nothing failed (there may be warnings); 1 a check failed;
2 usage or environment error.
`

const stepMigrate = "Run nerv migrate --dry-run to see the steps, then nerv migrate"

const stepMigrateUser = "Run nerv migrate --user --dry-run to see the steps, then nerv migrate --user"

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
	d.toolConfig()
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
	const step = stepMigrateUser
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
		runTime, err := workflowconfig.RunTimeUserKeys(doc, workflowconfig.Personal{})
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

// toolConfig reports a pending move of the user config.yaml's settings
// to the own file of a catalogue tool installed (S3): nerv-task's global
// task.yaml or nerv-release's global release.yaml. It adds nothing when
// none is pending, or when nerv migrate --user cannot plan (userConfig
// already reports a configuration that does not load).
func (d *doctorRun) toolConfig() {
	o := migrate.Options{Home: d.home, User: true, Now: d.opts.Now()}
	setToolFileTargets(d.ctx, d.opts, d.home, &o)
	if o.TaskYAMLHome == "" && o.ReleaseYAMLHome == "" {
		return
	}
	plan, err := migrate.Compute(o)
	if err != nil {
		return
	}
	for _, m := range pendingToolFileMoves(plan, o) {
		d.add("tool config", doctor.Warn, m.Tool+" is installed, but your user config.yaml still has its settings: nerv migrate --user moves them to "+m.Path, stepMigrateUser)
	}
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
	d.migration(top)
	d.projectOrphans(top)
	if !ok {
		return
	}
	wc := workflowconfig.Config{Project: project, User: d.user}
	scalar := func(key string) string {
		v, _ := wc.Scalar(key)
		return v
	}
	if origin != "" {
		d.add("origin", doctor.OK, origin, "")
	} else {
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
// projectOrphans reports the extension plugins nerv no longer ships that
// the repository's own Claude Code settings still name: nerv install and
// nerv upgrade clean the user settings only.
func (d *doctorRun) projectOrphans(top string) {
	var names, tools []string
	for _, t := range ecosystem.Catalogue() {
		for _, name := range t.Replaces {
			names = append(names, name+"@"+install.MarketplaceName)
		}
		tools = append(tools, t.Name)
	}
	var found []string
	for _, rel := range []string{".claude/settings.json", ".claude/settings.local.json"} {
		s, err := claude.LoadSettings(filepath.Join(top, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		var ids []string
		for _, id := range names {
			if s.RemovePlugin(id) {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			found = append(found, rel+": "+strings.Join(ids, ", "))
		}
	}
	if len(found) == 0 {
		return
	}
	d.add("old plugins", doctor.Warn, strings.Join(found, "; ")+" (nerv no longer ships them)",
		"Remove them from enabledPlugins in those files; "+strings.Join(tools, " and ")+" replace them (nerv install --tools "+strings.Join(tools, ",")+")")
}

func (d *doctorRun) migration(top string) {
	o := migrate.Options{RepoRoot: top, Home: d.home, Now: d.opts.Now()}
	setToolFileTargets(d.ctx, d.opts, d.home, &o)
	plan, err := migrate.Compute(o)
	switch {
	case err != nil:
		d.add("migration", doctor.Fail, "nerv migrate cannot run: "+err.Error(), "Fix what nerv migrate reports, then run it")
	case !plan.Empty() || plan.LegacyReleaseRepo:
		detail := fmt.Sprintf("nerv migrate has %d step(s) to take", len(plan.Changes))
		// S3: a pending task.yaml/release.yaml move is named.
		for i, m := range pendingToolFileMoves(plan, o) {
			if i == 0 {
				detail += ", including moving settings to "
			} else {
				detail += " and "
			}
			rel, _ := filepath.Rel(top, m.Path)
			detail += filepath.ToSlash(rel) + " (" + m.Tool + " is installed)"
		}
		// S5: an older nerv's sections are handed to nerv-release migrate.
		if plan.LegacyReleaseRepo {
			detail += ", then nerv-release migrate converts an older nerv's release and deploy configuration"
		}
		d.add("migration", doctor.Warn, detail, stepMigrate)
	default:
		d.add("migration", doctor.OK, "nothing to migrate", "")
	}
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
