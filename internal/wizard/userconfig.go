package wizard

import (
	"fmt"
	"io"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
	"github.com/war-apps/nerv-gentle-ai/internal/configure"
)

// teamworkField is one "-- tasks.providers.teamwork --" prompt: its
// managed-catalogue key and its prompt label.
type teamworkField struct{ key, label string }

var teamworkFields = []teamworkField{
	{"tasks.providers.teamwork.task_ref_prefix", "Task ref prefix"},
	{"tasks.providers.teamwork.assignee_id", "Assignee id"},
	{"tasks.providers.teamwork.default_project_id", "Default project id"},
	{"tasks.providers.teamwork.default_tasklist_id", "Default tasklist id"},
	{"tasks.providers.teamwork.stages.inDev", "Stage: in development"},
	{"tasks.providers.teamwork.stages.testing", "Stage: testing"},
	{"tasks.providers.teamwork.stages.implemented", "Stage: implemented"},
	{"tasks.providers.teamwork.stages.blocked", "Stage: blocked"},
	{"tasks.providers.teamwork.stages.canceled", "Stage: canceled"},
	{"tasks.providers.teamwork.stages.pending", "Stage: pending"},
	{"tasks.providers.teamwork.stages.analysis", "Stage: analysis"},
}

var skillsCategories = []string{"testing", "code", "best-practices", "architecture", "audit"}

// userConfigOrder is every key the user-config section can ask about, in
// config.Defaults' catalogue display order — the order the change summary
// and the --set batch use.
var userConfigOrder = func() []string {
	order := []string{
		"git.base_branch", "git.worktree", "git.branch_pattern", "git.commit_ref_pattern",
		"tasks.provider", "tasks.ask_when_missing", "tasks.subtasks_per_wave",
		"tasks.timer_store", "tasks.rounding_minutes",
	}
	for _, f := range teamworkFields {
		order = append(order, f.key)
	}
	for _, cat := range skillsCategories {
		order = append(order, "skills."+cat)
	}
	return append(order, "critical_paths", "artifacts.commit")
}()

// userConfigResult is runUserConfigSection's report: whether it wrote
// anything, plus the answered git.base_branch/tasks.provider (used as the
// Repos section's own defaults, exactly as configure.ps1's interactive
// body threads $gitBaseBranch/$tasksProvider into Section 3 regardless of
// whether Section 1 actually wrote anything).
type userConfigResult struct {
	Changed    bool
	BaseBranch string
	Provider   string
}

// runUserConfigSection is Section 1: asks git, tasks (+ the Teamwork
// provider sub-fields when tasks.provider resolves to "teamwork"), the
// five skills stacks, critical_paths, and artifacts.commit, then applies
// every actually-changed answer in one configure.Set batch — one write,
// one backup. Mirrors configure.ps1's "-- Section 1: User config --"
// (~1548-1971).
func runUserConfigSection(deps Deps, paths configure.Paths, s *session, out io.Writer) (userConfigResult, error) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- User config ---")

	doc, exists, err := (configure.Store{}).Load(paths.Config)
	if err != nil {
		return userConfigResult{}, err
	}
	fmt.Fprintf(out, "Config path    : %s %s\n", paths.Config, existsLabel(exists))

	current := func(key string) string {
		v, _ := config.ManagedValue(doc, key)
		return v
	}
	allowed := func(key string) []string {
		v, _ := config.AllowedValues(key)
		return v
	}

	answers := map[string]string{}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "-- git --")
	answers["git.base_branch"] = s.ask("Base branch", current("git.base_branch"))
	answers["git.worktree"] = s.choose("Worktree policy", allowed("git.worktree"), current("git.worktree"))
	answers["git.branch_pattern"] = s.ask("Branch pattern", current("git.branch_pattern"))
	answers["git.commit_ref_pattern"] = s.ask("Commit ref pattern", current("git.commit_ref_pattern"))

	fmt.Fprintln(out)
	fmt.Fprintln(out, "-- tasks --")
	answers["tasks.provider"] = s.choose("Task provider", allowed("tasks.provider"), current("tasks.provider"))
	answers["tasks.ask_when_missing"] = s.choose("Ask when missing", allowed("tasks.ask_when_missing"), current("tasks.ask_when_missing"))
	answers["tasks.subtasks_per_wave"] = s.choose("Subtasks per wave", allowed("tasks.subtasks_per_wave"), current("tasks.subtasks_per_wave"))
	answers["tasks.timer_store"] = s.ask("Timer store path", current("tasks.timer_store"))
	answers["tasks.rounding_minutes"] = s.ask("Rounding minutes", current("tasks.rounding_minutes"))

	if answers["tasks.provider"] == "teamwork" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "-- tasks.providers.teamwork --")
		for _, f := range teamworkFields {
			answers[f.key] = s.ask(f.label, current(f.key))
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "-- skills (comma-separated) --")
	for _, cat := range skillsCategories {
		key := "skills." + cat
		answers[key] = s.ask(cat, current(key))
	}

	fmt.Fprintln(out)
	answers["critical_paths"] = s.ask("Critical paths (comma-separated)", current("critical_paths"))

	fmt.Fprintln(out)
	answers["artifacts.commit"] = s.choose("Artifacts commit policy", allowed("artifacts.commit"), current("artifacts.commit"))

	result := userConfigResult{BaseBranch: answers["git.base_branch"], Provider: answers["tasks.provider"]}

	var changeLog []string
	var setArgs []string
	for _, key := range userConfigOrder {
		newVal, asked := answers[key]
		if !asked {
			continue
		}
		oldVal := current(key)
		if newVal == oldVal {
			continue
		}
		changeLog = append(changeLog, fmt.Sprintf("%s: %s -> %s", key, oldVal, newVal))
		setArgs = append(setArgs, key+"="+newVal)
	}

	fmt.Fprintln(out)
	if len(changeLog) == 0 {
		fmt.Fprintf(out, "No changes; %s left untouched.\n", paths.Config)
		return result, nil
	}

	fmt.Fprintln(out, "--- Summary ---")
	for _, c := range changeLog {
		fmt.Fprintf(out, "  %s\n", c)
	}

	if !s.yesNo(fmt.Sprintf("Write to %s?", paths.Config), true) {
		fmt.Fprintln(out, "Aborted; no changes written to user config.")
		return result, nil
	}

	setResult, err := configure.Set(deps, paths, setArgs)
	if err != nil {
		return result, err
	}
	if setResult.Backup != nil {
		fmt.Fprintf(out, "Backup written: %s\n", *setResult.Backup)
	}
	if setResult.Changed {
		fmt.Fprintf(out, "Written: %s\n", paths.Config)
	}
	result.Changed = setResult.Changed
	return result, nil
}

func existsLabel(exists bool) string {
	if exists {
		return "(exists)"
	}
	return "(will be created)"
}
