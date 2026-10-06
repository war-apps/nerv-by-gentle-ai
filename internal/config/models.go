package config

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// ModelOverride is one role's { model, effort, from } override entry as
// written in (or resolved for) a document's models: block. An empty field
// means it is absent — From, Model and From are never legitimately the
// empty string.
type ModelOverride struct {
	Model  string
	Effort string
	From   string
}

// RoleInfo describes one role for display: what it does, which gentle-ai
// v4 agent it is comparable to, and which gentle-ai phase to suggest for a
// from:<phase> override. Neither ever changes how a role's model is
// resolved (from:<phase> stays explicit).
type RoleInfo struct {
	// Purpose is a one-line description of what the role does.
	Purpose string
	// GentleAIEquivalent is the closest gentle-ai v4 agent (a jd-judge or a
	// native review agent), or "" when the role has none. It is
	// informational only and never a from:<phase> value: native review
	// agents are not claude_phase_assignments keys.
	GentleAIEquivalent string
	// FromPhase is the gentle-ai claude_phase_assignments key to suggest as
	// from:<phase> for this role, or "" when no phase fits.
	FromPhase string
}

// RoleCatalogue is the 19 NERV agent roles, their group shortcuts and the
// display metadata for both.
type RoleCatalogue struct {
	AllRoles []string
	Groups   map[string][]string
	// Info maps every role in AllRoles to its RoleInfo.
	Info map[string]RoleInfo
	// GroupDescriptions maps every group in Groups to a short legend.
	GroupDescriptions map[string]string
}

// Roles returns the role catalogue — the single source of truth shared by
// the interactive wizard's role/group prompts and "nerv configure
// --set-model"'s role validation.
//
// It returns role names, group membership and display metadata (purpose,
// gentle-ai equivalent), not a per-role plugin default model/effort — those live in plugin/agents/*.md frontmatter on
// disk, read by internal/models (out of this file-I/O-free package's
// scope). ModelTable below takes the resolved defaults as a parameter
// instead.
func Roles() RoleCatalogue {
	allRoles := []string{
		"aoba", "asuka", "balthasar", "casper", "fuyutsuki", "hyuga", "kaji",
		"kaji-coverage", "kaji-refuter", "kaji-resilience", "kaji-security",
		"kaworu", "maya", "melchor", "misato", "rei", "ritsuko", "shinji", "toji",
	}
	groups := map[string][]string{
		"magi":        {"balthasar", "melchor", "casper"},
		"pilots":      {"rei", "shinji", "asuka", "toji", "kaworu"},
		"kaji-passes": {"kaji", "kaji-security", "kaji-coverage", "kaji-resilience", "kaji-refuter"},
		"all":         allRoles,
	}
	info := map[string]RoleInfo{
		"misato":          {"authors the plan (proposal, design, tasks)", "", ""},
		"ritsuko":         {"intelligence, test planning, end-of-run docs", "", ""},
		"hyuga":           {"task criticality, dependency waves, tracking", "", ""},
		"melchor":         {"MAGI vote: structure and security", "jd-judge-b", "jd-judge-b"},
		"balthasar":       {"MAGI vote: software principles, readability", "jd-judge-a", "jd-judge-a"},
		"casper":          {"MAGI vote: process and documentation", "jd-judge-a", "jd-judge-a"},
		"fuyutsuki":       {"governance veto on new skills/scripts/commands", "", ""},
		"kaworu":          {"writes the failing tests first", "", ""},
		"shinji":          {"backend pilot", "", ""},
		"asuka":           {"frontend pilot", "", ""},
		"rei":             {"data pilot (persistence, observability)", "", ""},
		"toji":            {"infrastructure pilot (CI/CD, containers)", "", ""},
		"maya":            {"quality gate (tests, lint, build)", "", ""},
		"kaji":            {"audit compiler", "", ""},
		"kaji-security":   {"audit pass: security", "review-risk", ""},
		"kaji-coverage":   {"audit pass: test coverage, reliability, correctness", "review-reliability", ""},
		"kaji-resilience": {"audit pass: resilience and performance", "review-resilience", ""},
		"kaji-refuter":    {"refutes severe audit findings", "review-refuter", ""},
		"aoba":            {"commits, PRs and run telemetry", "", ""},
	}
	groupDescriptions := map[string]string{
		"magi":        "the three voters (balthasar, melchor, casper)",
		"pilots":      "the implementers (kaworu, shinji, asuka, rei, toji)",
		"kaji-passes": "the audit passes (kaji, kaji-security, kaji-coverage, kaji-resilience, kaji-refuter)",
		"all":         "every role",
	}
	return RoleCatalogue{AllRoles: allRoles, Groups: groups, Info: info, GroupDescriptions: groupDescriptions}
}

// ErrUnknownRole is returned by ValidateRole for a role outside the
// catalogue.
type ErrUnknownRole struct {
	Role string
}

func (e *ErrUnknownRole) Error() string {
	return fmt.Sprintf("Unknown role '%s'. Valid roles: %s.", e.Role, strings.Join(Roles().AllRoles, ", "))
}

// ValidateRole reports an *ErrUnknownRole when role is not one of the 19
// catalogue roles.
func ValidateRole(role string) error {
	for _, r := range Roles().AllRoles {
		if r == role {
			return nil
		}
	}
	return &ErrUnknownRole{Role: role}
}

// ResolveRoleTarget resolves one role-selection answer from the
// interactive models wizard — a role name, a group name, or a 1-based row
// number — into the role names it designates, or nil when it matches
// none. Mirrors Resolve-NervRoleTarget.
func ResolveRoleTarget(target string, allRoles []string, groups map[string][]string, numberMap map[string]string) []string {
	t := strings.TrimSpace(target)
	if t == "" {
		return nil
	}
	lower := strings.ToLower(t)
	if g, ok := groups[lower]; ok {
		return g
	}
	if role, ok := numberMap[t]; ok {
		return []string{role}
	}
	for _, r := range allRoles {
		if r == lower {
			return []string{r}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// FormatModelsBlock / SetModelsBlock / ReadModelsOverrides
// ---------------------------------------------------------------------------

// FormatModelsBlock renders a models: block from a role -> override map:
// roles sorted, one inline-map line per role, LF line endings, no trailing
// blank line, preceded by a documented header comment. Mirrors
// Format-NervModelsBlock.
func FormatModelsBlock(overrides map[string]ModelOverride) string {
	lines := []string{"models:                             # per-role model and effort (written by nerv configure --set-model)"}

	roles := make([]string, 0, len(overrides))
	for role := range overrides {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	for _, role := range roles {
		entry := overrides[role]
		var parts []string
		if entry.From != "" {
			parts = append(parts, "from: "+entry.From)
		}
		if entry.Model != "" {
			parts = append(parts, "model: "+entry.Model)
		}
		if entry.Effort != "" {
			parts = append(parts, "effort: "+entry.Effort)
		}
		if len(parts) == 0 {
			continue
		}
		lines = append(lines, "  "+role+": { "+strings.Join(parts, ", ")+" }")
	}

	return strings.Join(lines, "\n")
}

// SetModelsBlock replaces, appends, or removes doc's top-level models:
// block with blockText, the same way Document.SetBlock does for any other
// top-level key. Mirrors Set-NervYamlModelsBlock — whose scan is a strict
// subset of the generic Set-NervYamlBlock's (it does not recognize a
// block-scalar indicator on the models: line itself, which never occurs in
// practice), so the two always agree; see
// TestSetModelsBlock_DelegatesToGenericSetBlock.
func SetModelsBlock(doc *Document, blockText string) {
	doc.SetBlock("models", blockText)
}

var modelEntryRe = regexp.MustCompile(`^\s*([A-Za-z0-9_-]+):\s*\{([^}]*)\}\s*(#.*)?$`)

// ReadModelsOverrides parses doc's models: block into a role -> override
// map, keeping fields exactly as written (an unresolved from: phase name,
// not the model/effort it resolves to) — a raw scan only, with no
// cross-check against the resolved, file-sourced phase assignment table
// (that needs filesystem access, out of this package's scope). ModelTable
// below takes the resolved phase assignments as a parameter so callers
// can apply that display-time resolution themselves.
func ReadModelsOverrides(doc *Document) map[string]ModelOverride {
	result := map[string]ModelOverride{}

	blockText, found := doc.Block("models")
	if !found {
		return result
	}

	for _, line := range strings.Split(blockText, "\n") {
		m := modelEntryRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		role, body := m[1], m[2]
		entry := ModelOverride{}
		for _, pair := range strings.Split(body, ",") {
			if strings.TrimSpace(pair) == "" {
				continue
			}
			kv := strings.SplitN(pair, ":", 2)
			if len(kv) != 2 {
				continue
			}
			key := strings.TrimSpace(kv[0])
			value := trimQuotes(strings.TrimSpace(kv[1]))
			switch key {
			case "from":
				entry.From = value
			case "model":
				entry.Model = value
			case "effort":
				entry.Effort = value
			}
		}
		if entry != (ModelOverride{}) {
			result[role] = entry
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// ModelTable
// ---------------------------------------------------------------------------

// ModelRow is one row of the resolved model/effort table that
// "nerv configure --print" emits.
type ModelRow struct {
	Role   string
	Model  string
	Effort string
	// Source is "default", "override", "gentle-ai:<phase>", or
	// "gentle-ai:<phase>" + MissingPhaseSuffix when the phase is absent
	// from a known, non-empty set of phase assignments.
	Source string
}

// PhaseAssignment is one gentle-ai claude_phase_assignments entry (model
// and effort for a named phase), used to resolve a role's from:<phase>
// override for display.
type PhaseAssignment struct {
	Model  string
	Effort string
}

// MissingPhaseSuffix is appended to a "gentle-ai:<phase>" source when the
// phase is not a key of a known, non-empty phase-assignment map, so the
// row does not claim a gentle-ai resolution it never got.
const MissingPhaseSuffix = " (missing; plugin default)"

// ModelTable builds the display table (role, model, effort, source) from
// the plugin default model/effort per role, the raw models: overrides, and
// gentle-ai's phase assignments (for from:<phase> display). source is
// "override" when the role has an explicit model or effort key in
// overrides, "gentle-ai:<phase>" when it only has a from key, or "default"
// when it has no override at all. A from:<phase> missing from a non-empty
// phaseAssignments gets MissingPhaseSuffix and keeps the plugin default;
// a nil or empty phaseAssignments means the assignments are unknown
// (state.json absent or unreadable), so nothing is flagged. A role present in overrides but not
// defaults still appears. Mirrors Get-NervModelTable.
func ModelTable(defaults, overrides map[string]ModelOverride, phaseAssignments map[string]PhaseAssignment) []ModelRow {
	roleSet := map[string]struct{}{}
	for role := range defaults {
		roleSet[role] = struct{}{}
	}
	for role := range overrides {
		roleSet[role] = struct{}{}
	}
	roles := make([]string, 0, len(roleSet))
	for role := range roleSet {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	rows := make([]ModelRow, 0, len(roles))
	for _, role := range roles {
		model, effort := defaults[role].Model, defaults[role].Effort
		source := "default"

		if override, ok := overrides[role]; ok {
			if override.From != "" {
				source = "gentle-ai:" + override.From
				if pa, ok := phaseAssignments[override.From]; ok {
					if pa.Model != "" {
						model = pa.Model
					}
					if pa.Effort != "" {
						effort = pa.Effort
					}
				} else if len(phaseAssignments) > 0 {
					source += MissingPhaseSuffix
				}
			}
			if override.Model != "" {
				model = override.Model
				source = "override"
			}
			if override.Effort != "" {
				effort = override.Effort
				source = "override"
			}
		}

		rows = append(rows, ModelRow{Role: role, Model: model, Effort: effort, Source: source})
	}

	return rows
}

// ModelTableFromDocument is ModelTable's convenience form: it reads doc's
// raw overrides itself (ReadModelsOverrides) instead of requiring the
// caller to do so first.
func ModelTableFromDocument(doc *Document, defaults map[string]ModelOverride, phaseAssignments map[string]PhaseAssignment) []ModelRow {
	return ModelTable(defaults, ReadModelsOverrides(doc), phaseAssignments)
}

// ---------------------------------------------------------------------------
// ResolveModelSpec
// ---------------------------------------------------------------------------

// ErrInvalidModel is returned by ResolveModelSpec for a model token that is
// neither a known alias nor a claude-... id.
type ErrInvalidModel struct {
	Role  string
	Model string
}

func (e *ErrInvalidModel) Error() string {
	return fmt.Sprintf("Invalid model '%s' for role '%s'. Allowed: sonnet, opus, haiku, fable, inherit, a claude-... id, from:<phase>, or default.", e.Model, e.Role)
}

// ErrInvalidEffort is returned by ResolveModelSpec for an effort token
// outside the known set.
type ErrInvalidEffort struct {
	Role   string
	Effort string
}

func (e *ErrInvalidEffort) Error() string {
	return fmt.Sprintf("Invalid effort '%s' for role '%s'. Allowed: low, medium, high, xhigh, max.", e.Effort, e.Role)
}

var (
	customModelRe = regexp.MustCompile(`^claude-.+$`)
	fromSpecRe    = regexp.MustCompile(`(?i)^from:(.+)$`)
)

// ModelAliases returns the known per-role model alias tokens, in
// canonical menu order.
func ModelAliases() []string {
	return []string{"sonnet", "opus", "haiku", "fable", "inherit"}
}

// Efforts returns the known per-role effort tokens, in canonical menu
// order.
func Efforts() []string {
	return []string{"low", "medium", "high", "xhigh", "max"}
}

// IsCustomModelID reports whether s looks like a raw "claude-..." model
// id rather than one of ModelAliases()'s known aliases.
func IsCustomModelID(s string) bool {
	return customModelRe.MatchString(s)
}

// ResolveModelSpec parses one "--set-model role=<spec>" value —
// model[/effort] (a model alias — sonnet, opus, haiku, fable, inherit —
// or a claude-... id, optionally with an effort of
// low|medium|high|xhigh|max), from:<phase>, or default (case-insensitive,
// clears the role's override) — into the override to apply for role and
// whether it clears the override entirely. role is only used to format an
// error message.
func ResolveModelSpec(role, spec string) (override ModelOverride, clear bool, err error) {
	if strings.EqualFold(spec, "default") {
		return ModelOverride{}, true, nil
	}
	if m := fromSpecRe.FindStringSubmatch(spec); m != nil {
		return ModelOverride{From: m[1]}, false, nil
	}

	parts := strings.SplitN(spec, "/", 2)
	modelPart := parts[0]
	var effortPart string
	if len(parts) > 1 {
		effortPart = parts[1]
	}

	if !slices.Contains(ModelAliases(), modelPart) && !IsCustomModelID(modelPart) {
		return ModelOverride{}, false, &ErrInvalidModel{Role: role, Model: modelPart}
	}
	if effortPart != "" && !slices.Contains(Efforts(), effortPart) {
		return ModelOverride{}, false, &ErrInvalidEffort{Role: role, Effort: effortPart}
	}

	return ModelOverride{Model: modelPart, Effort: effortPart}, false, nil
}
