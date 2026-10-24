package config

import "strings"

// KeyDefault pairs one managed nerv.yaml key with its built-in default
// value, in the catalogue's canonical order.
type KeyDefault struct {
	Key   string
	Value string
}

// SkillsCategories returns the skills: block's consuming-role categories,
// in the canonical order every skills prompt list and block uses.
func SkillsCategories() []string {
	return []string{"testing", "code", "best-practices", "architecture", "audit"}
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
		"  architecture: [" + v.Architecture + "]          # melchior",
		"  audit: [" + v.Audit + "]                       # audit passes, melchior audit",
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

	lines = append(lines, "", "# models:                           # optional: per-role model/effort overrides for this repo (see nerv configure --init-repo)")

	return strings.Join(lines, "\n") + "\n"
}
