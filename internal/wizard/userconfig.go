package wizard

import (
	"fmt"
	"io"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
)

var skillsCategories = config.SkillsCategories()

// userConfigOrder is every key the user-config section can ask about, in
// config.Defaults' catalogue display order — the order the change summary
// and the --set batch use.
var userConfigOrder = func() []string {
	order := []string{
		"git.base_branch", "git.worktree", "git.worktree_pattern", "git.branch_pattern", "git.commit_ref_pattern",
	}
	for _, cat := range skillsCategories {
		order = append(order, "skills."+cat)
	}
	return append(order, "critical_paths", "artifacts.commit")
}()

// userConfigResult is runUserConfigSection's report: whether it wrote
// anything, plus the answered git.base_branch (threaded into the Repos
// section as its own default, regardless of whether Section 1 actually
// wrote anything).
type userConfigResult struct {
	Changed    bool
	BaseBranch string
}

// runUserConfigSection is Section 1: asks git, the five skills stacks,
// critical_paths, and artifacts.commit, then applies every actually-changed
// answer in one configure.Set batch — one write, one backup. Task settings
// are not core's: the Task providers section asks them from the provider
// manifests.
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
	if sectionErr == nil {
		v, err := worktreePatternField(deps, s, out, current("git.worktree_pattern"))
		if err != nil {
			sectionErr = err
		} else {
			answers["git.worktree_pattern"] = v
		}
	}
	askField("Branch pattern", "git.branch_pattern")
	askField("Commit ref pattern", "git.commit_ref_pattern")

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

	result := userConfigResult{BaseBranch: answers["git.base_branch"]}

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
