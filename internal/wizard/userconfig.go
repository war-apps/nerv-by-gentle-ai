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

// teamworkStageLabels is each Teamwork stage's prompt label, keyed by
// config.StageOrder's stage name.
var teamworkStageLabels = map[string]string{
	"inDev":       "Stage: in development",
	"testing":     "Stage: testing",
	"implemented": "Stage: implemented",
	"blocked":     "Stage: blocked",
	"canceled":    "Stage: canceled",
	"pending":     "Stage: pending",
	"analysis":    "Stage: analysis",
}

var teamworkFields = buildTeamworkFields()

func buildTeamworkFields() []teamworkField {
	fields := []teamworkField{
		{"tasks.providers.teamwork.task_ref_prefix", "Task ref prefix"},
		{"tasks.providers.teamwork.assignee_id", "Assignee id"},
		{"tasks.providers.teamwork.default_project_id", "Default project id"},
		{"tasks.providers.teamwork.default_tasklist_id", "Default tasklist id"},
	}
	for _, stage := range config.StageOrder() {
		fields = append(fields, teamworkField{"tasks.providers.teamwork.stages." + stage, teamworkStageLabels[stage]})
	}
	return fields
}

var skillsCategories = config.SkillsCategories()

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
// anything, plus the answered git.base_branch/tasks.provider (threaded
// into Section 3 as the Repos section's own defaults, regardless of
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
// one backup.
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

	// ask/choose wrappers that stop the whole section at the first
	// ErrInputClosed: once the reader is genuinely exhausted, no further
	// prompt is asked and no write is attempted (all-or-nothing — see
	// ErrInputClosed's doc comment).
	var sectionErr error
	askField := func(label, key string) {
		if sectionErr != nil {
			return
		}
		v, err := s.ask(label, current(key))
		if err != nil {
			sectionErr = err
			return
		}
		answers[key] = v
	}
	chooseField := func(label, key string) {
		if sectionErr != nil {
			return
		}
		v, err := s.choose(label, allowed(key), current(key))
		if err != nil {
			sectionErr = err
			return
		}
		answers[key] = v
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "-- git --")
	askField("Base branch", "git.base_branch")
	chooseField("Worktree policy", "git.worktree")
	askField("Branch pattern", "git.branch_pattern")
	askField("Commit ref pattern", "git.commit_ref_pattern")

	fmt.Fprintln(out)
	fmt.Fprintln(out, "-- tasks --")
	chooseField("Task provider", "tasks.provider")
	chooseField("Ask when missing", "tasks.ask_when_missing")
	chooseField("Subtasks per wave", "tasks.subtasks_per_wave")
	askField("Timer store path", "tasks.timer_store")
	askField("Rounding minutes", "tasks.rounding_minutes")

	if sectionErr == nil && answers["tasks.provider"] == "teamwork" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "-- tasks.providers.teamwork --")
		for _, f := range teamworkFields {
			askField(f.label, f.key)
		}
	}

	if sectionErr == nil {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "-- skills (comma-separated) --")
		for _, cat := range skillsCategories {
			askField(cat, "skills."+cat)
		}
	}

	if sectionErr == nil {
		fmt.Fprintln(out)
		askField("Critical paths (comma-separated)", "critical_paths")
	}

	if sectionErr == nil {
		fmt.Fprintln(out)
		chooseField("Artifacts commit policy", "artifacts.commit")
	}

	if sectionErr != nil {
		return userConfigResult{}, sectionErr
	}

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

	write, err := s.yesNo(fmt.Sprintf("Write to %s?", paths.Config), true)
	if err != nil {
		return userConfigResult{}, err
	}
	if !write {
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
