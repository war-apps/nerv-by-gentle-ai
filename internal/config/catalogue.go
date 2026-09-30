package config

import "strings"

// KeyDefault pairs one managed nerv.yaml key with its built-in default
// value, in the catalogue's canonical order.
type KeyDefault struct {
	Key   string
	Value string
}

// StageOrder returns the Teamwork workflow stage names, in the canonical
// order every stages: block and prompt list uses.
func StageOrder() []string {
	return []string{"inDev", "testing", "implemented", "blocked", "canceled", "pending", "analysis"}
}

// SkillsCategories returns the skills: block's consuming-role categories,
// in the canonical order every skills prompt list and block uses.
func SkillsCategories() []string {
	return []string{"testing", "code", "best-practices", "architecture", "audit"}
}

// stageDefaults holds each Teamwork stage's built-in default value, keyed
// by StageOrder's stage name.
var stageDefaults = map[string]string{
	"inDev":       "DESARROLLO",
	"testing":     "TESTING",
	"implemented": "IMPLEMENTA",
	"blocked":     "BLOQUEA",
	"canceled":    "CANCEL",
	"pending":     "PENDIENTE",
	"analysis":    "ANALISIS",
}

// skillsDefaults holds each skills category's built-in default stack,
// keyed by SkillsCategories' category name.
var skillsDefaults = map[string]string{
	"testing":        "tdd, playwright-best-practices",
	"code":           "dotnet-best-practices, typescript-best-practices",
	"best-practices": "best-practices, solid-principles, clean-code-guard",
	"architecture":   "hexagonal-architecture, c4-architecture",
	"audit":          "security-review, clean-code-guard",
}

// Defaults returns the built-in default for every managed nerv.yaml key, in
// catalogue order — the complete catalogue of keys SetManagedValue accepts.
// Mirrors Get-NervConfigDefaults.
func Defaults() []KeyDefault {
	defaults := []KeyDefault{
		{"git.base_branch", "develop"},
		{"git.worktree", "ask"},
		{"git.worktree_pattern", ".claude/worktrees/{slug}"},
		{"git.branch_pattern", "feature/{prefix}-{id}-{slug}"},
		{"git.commit_ref_pattern", "({PREFIX}-{id})"},
		{"tasks.provider", "teamwork"},
		{"tasks.ask_when_missing", "true"},
		{"tasks.subtasks_per_wave", "false"},
		{"tasks.timer_store", "~/.claude/work/timers.json"},
		{"tasks.rounding_minutes", "15"},
		{"tasks.providers.teamwork.task_ref_prefix", "tw"},
		{"tasks.providers.teamwork.assignee_id", ""},
		{"tasks.providers.teamwork.default_project_id", ""},
		{"tasks.providers.teamwork.default_tasklist_id", ""},
	}
	for _, stage := range StageOrder() {
		defaults = append(defaults, KeyDefault{"tasks.providers.teamwork.stages." + stage, stageDefaults[stage]})
	}
	for _, cat := range SkillsCategories() {
		defaults = append(defaults, KeyDefault{"skills." + cat, skillsDefaults[cat]})
	}
	defaults = append(defaults,
		KeyDefault{"critical_paths", "auth/, payments/, migrations/, infra/"},
		KeyDefault{"artifacts.commit", "at-close"},
	)
	return defaults
}

// DefaultValue returns key's built-in default and whether key is managed.
func DefaultValue(key string) (value string, ok bool) {
	for _, kd := range Defaults() {
		if kd.Key == key {
			return kd.Value, true
		}
	}
	return "", false
}

// ManagedKeys returns every managed key, in catalogue order — the same
// order and wording used in the "unknown config key" error message.
func ManagedKeys() []string {
	defaults := Defaults()
	keys := make([]string, len(defaults))
	for i, kd := range defaults {
		keys[i] = kd.Key
	}
	return keys
}

// AllowedValues returns the closed set of valid values for a managed key
// that has one, and whether key is enumerated at all. A managed key absent
// from this catalogue (ok == false) accepts any value. Mirrors
// Get-NervConfigAllowedValues.
func AllowedValues(key string) (values []string, ok bool) {
	switch key {
	case "git.worktree":
		return []string{"ask", "always", "never"}, true
	case "tasks.provider":
		return []string{"teamwork", "github-projects", "jira", "none"}, true
	case "tasks.ask_when_missing":
		return []string{"true", "false"}, true
	case "tasks.subtasks_per_wave":
		return []string{"true", "false"}, true
	case "artifacts.commit":
		return []string{"with-change", "at-close", "never"}, true
	default:
		return nil, false
	}
}

// ---------------------------------------------------------------------------
// Block formatters (Format-NervSkillsBlock, Format-NervCriticalPathsLine,
// Format-NervProjectFile) — render text byte-identical to the PowerShell
// functions' documented-syntax output.
// ---------------------------------------------------------------------------

// SkillsValues holds the fields Format-NervSkillsBlock's caller supplies.
type SkillsValues struct {
	Testing       string
	Code          string
	BestPractices string
	Architecture  string
	Audit         string
}

// FormatSkillsBlock renders the skills: block in the documented syntax.
// Mirrors Format-NervSkillsBlock.
func FormatSkillsBlock(v SkillsValues) string {
	lines := []string{
		"skills:                             # stacks per consuming role; names must exist in .atl/skill-registry.md",
		"  testing: [" + v.Testing + "]                        # ritsuko, kaworu, maya",
		"  code: [" + v.Code + "]         # pilots",
		"  best-practices: [" + v.BestPractices + "]  # balthasar",
		"  architecture: [" + v.Architecture + "]          # melchor",
		"  audit: [" + v.Audit + "]                       # kaji passes",
	}
	return strings.Join(lines, "\n")
}

// FormatCriticalPathsLine renders the single-line critical_paths: entry in
// the documented syntax. Mirrors Format-NervCriticalPathsLine.
func FormatCriticalPathsLine(paths []string) string {
	return "critical_paths: [" + strings.Join(paths, ", ") + "]            # Hyuga auto-critical"
}

// ProjectValues holds the optional overrides Format-NervProjectFile's
// caller supplies; an empty field is omitted from the rendered file.
type ProjectValues struct {
	BaseBranch string
	Provider   string
	ProjectID  string
	TasklistID string
}

// FormatProjectFile renders a project-scope .nerv/nerv.yaml text:
// enabled: true, then only the fields the caller set, each with a short
// comment, plus a commented models: hint line. Mirrors
// Format-NervProjectFile.
func FormatProjectFile(v ProjectValues) string {
	lines := []string{"enabled: true"}

	if v.BaseBranch != "" {
		lines = append(lines, "", "git:",
			"  base_branch: "+v.BaseBranch+"              # overrides the user-scope default for this repo")
	}

	hasProvider := v.Provider != ""
	hasProjectID := v.ProjectID != ""
	hasTasklistID := v.TasklistID != ""
	if hasProvider || hasProjectID || hasTasklistID {
		lines = append(lines, "", "tasks:")
		if hasProvider {
			lines = append(lines, "  provider: "+v.Provider+"                # overrides the user-scope default for this repo")
		}
		if hasProjectID || hasTasklistID {
			lines = append(lines, "  providers:", "    teamwork:")
			if hasProjectID {
				lines = append(lines, "      project_id: "+v.ProjectID+"                # this repo's Teamwork project")
			}
			if hasTasklistID {
				lines = append(lines, "      tasklist_id: "+v.TasklistID+"                # this repo's Teamwork tasklist")
			}
		}
	}

	lines = append(lines, "", "# models:                           # optional: per-role model/effort overrides for this repo (see nerv configure --init-repo)")

	return strings.Join(lines, "\n") + "\n"
}
