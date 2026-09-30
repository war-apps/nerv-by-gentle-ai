package config

import "strings"

// KeyDefault pairs one managed nerv.yaml key with its built-in default
// value, in the catalogue's canonical order.
type KeyDefault struct {
	Key   string
	Value string
}

// Defaults returns the built-in default for every managed nerv.yaml key, in
// catalogue order — the complete catalogue of keys SetManagedValue accepts.
// Mirrors Get-NervConfigDefaults.
func Defaults() []KeyDefault {
	return []KeyDefault{
		{"git.base_branch", "develop"},
		{"git.worktree", "ask"},
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
		{"tasks.providers.teamwork.stages.inDev", "DESARROLLO"},
		{"tasks.providers.teamwork.stages.testing", "TESTING"},
		{"tasks.providers.teamwork.stages.implemented", "IMPLEMENTA"},
		{"tasks.providers.teamwork.stages.blocked", "BLOQUEA"},
		{"tasks.providers.teamwork.stages.canceled", "CANCEL"},
		{"tasks.providers.teamwork.stages.pending", "PENDIENTE"},
		{"tasks.providers.teamwork.stages.analysis", "ANALISIS"},
		{"skills.testing", "tdd, playwright-best-practices"},
		{"skills.code", "dotnet-best-practices, typescript-best-practices"},
		{"skills.best-practices", "best-practices, solid-principles, clean-code-guard"},
		{"skills.architecture", "hexagonal-architecture, c4-architecture"},
		{"skills.audit", "security-review, clean-code-guard"},
		{"critical_paths", "auth/, payments/, migrations/, infra/"},
		{"artifacts.commit", "at-close"},
	}
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
// Block formatters (Format-NervGitBlock, Format-NervTasksBlock,
// Format-NervSkillsBlock, Format-NervCriticalPathsLine,
// Format-NervArtifactsBlock, Format-NervProjectFile) — render text
// byte-identical to the PowerShell functions' documented-syntax output.
// ---------------------------------------------------------------------------

// GitValues holds the fields Format-NervGitBlock's caller supplies.
type GitValues struct {
	BaseBranch       string
	Worktree         string
	BranchPattern    string
	CommitRefPattern string
}

// FormatGitBlock renders the git: block in the documented syntax. Mirrors
// Format-NervGitBlock.
func FormatGitBlock(v GitValues) string {
	lines := []string{
		"git:",
		"  base_branch: " + v.BaseBranch + "              # default base for the worktree offer",
		"  worktree: " + v.Worktree + "                     # ask | always | never",
		`  branch_pattern: "` + v.BranchPattern + `"   # prefix comes from the provider (tw, gh, jira)`,
		`  commit_ref_pattern: "` + v.CommitRefPattern + `"`,
	}
	return strings.Join(lines, "\n")
}

// TasksValues holds the fields Format-NervTasksBlock's caller supplies.
type TasksValues struct {
	Provider                  string
	AskWhenMissing            string
	SubtasksPerWave           string
	TimerStore                string
	RoundingMinutes           string
	TeamworkTaskRefPrefix     string
	TeamworkAssigneeID        string
	TeamworkDefaultProjectID  string
	TeamworkDefaultTasklistID string
	TeamworkStageInDev        string
	TeamworkStageTesting      string
	TeamworkStageImplemented  string
	TeamworkStageBlocked      string
	TeamworkStageCanceled     string
	TeamworkStagePending      string
	TeamworkStageAnalysis     string
}

// FormatTasksBlock renders the tasks: block in the documented syntax,
// re-emitting providers.teamwork from v and keeping
// providers.github-projects / providers.jira as the documented inline-map
// placeholders. When sourcesRaw is non-empty, its raw sources: sub-block
// text (as returned by SubBlock) is appended verbatim. Mirrors
// Format-NervTasksBlock.
func FormatTasksBlock(v TasksValues, sourcesRaw string) string {
	stages := "inDev: " + v.TeamworkStageInDev +
		", testing: " + v.TeamworkStageTesting +
		", implemented: " + v.TeamworkStageImplemented +
		", blocked: " + v.TeamworkStageBlocked +
		", canceled: " + v.TeamworkStageCanceled +
		", pending: " + v.TeamworkStagePending +
		", analysis: " + v.TeamworkStageAnalysis

	lines := []string{
		"tasks:",
		`  provider: ` + v.Provider + `                # teamwork | github-projects | jira | none ; "ask" when absent`,
		"  ask_when_missing: " + v.AskWhenMissing + "            # preflight asks task + worktree + branch if no active task",
		"  subtasks_per_wave: " + v.SubtasksPerWave,
		"  timer_store: " + v.TimerStore,
		"  rounding_minutes: " + v.RoundingMinutes,
		"  providers:                        # one block per provider, only the enabled one is required",
		"    teamwork:",
		"      task_ref_prefix: " + v.TeamworkTaskRefPrefix + "           # {prefix} in branch_pattern / commit_ref_pattern",
		"      assignee_id: " + v.TeamworkAssigneeID + "           # user scope",
		"      default_project_id: " + v.TeamworkDefaultProjectID,
		"      default_tasklist_id: " + v.TeamworkDefaultTasklistID,
		"      stages: { " + stages + " }",
		`    github-projects: { task_ref_prefix: gh, owner: "", project_number: 0 }    # later`,
		`    jira: { task_ref_prefix: jira, site: "", project_key: "" }               # later`,
	}

	if sourcesRaw != "" {
		lines = append(lines, yamlSplitLines(sourcesRaw)...)
	}

	return strings.Join(lines, "\n")
}

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

// FormatArtifactsBlock renders the artifacts: block in the documented
// syntax. Mirrors Format-NervArtifactsBlock.
func FormatArtifactsBlock(commit string) string {
	return "artifacts:\n  commit: " + commit + "                  # with-change | at-close | never (default: at-close)"
}

// ProjectValues holds the optional overrides Format-NervProjectFile's
// caller supplies; an empty field is omitted from the rendered file.
type ProjectValues struct {
	BaseBranch string
	Provider   string
	ProjectID  string
	TasklistID string
	Commit     string
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

	if v.Commit != "" {
		lines = append(lines, "", "artifacts:",
			"  commit: "+v.Commit+"                  # with-change | at-close | never")
	}

	lines = append(lines, "", "# models:                           # optional: per-role model/effort overrides for this repo (see tools/configure-models.ps1 -Scope project)")

	return strings.Join(lines, "\n") + "\n"
}
