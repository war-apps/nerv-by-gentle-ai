package config

import (
	"fmt"
	"regexp"
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

// RoleCatalogue is the 18 NERV agent roles and their group shortcuts.
type RoleCatalogue struct {
	AllRoles []string
	Groups   map[string][]string
}

// Roles returns the role catalogue — the single source of truth shared by
// the interactive wizard's role/group prompts and configure.ps1's
// -SetModel role validation. Mirrors Get-NervRoleCatalogue.
//
// Deviation from the port brief: the PowerShell function returns only role
// names and group membership, not a per-role plugin default model/effort —
// those come from Get-NervPluginDefaults (tools/install.ps1), which reads
// plugin/agents/*.md frontmatter off disk and is out of this
// file-I/O-free package's scope (P2's internal/models). ModelTable below
// takes the resolved defaults as a parameter instead.
func Roles() RoleCatalogue {
	allRoles := []string{
		"aoba", "asuka", "balthasar", "casper", "fuyutsuki", "hyuga", "kaji",
		"kaji-coverage", "kaji-refuter", "kaji-security", "kaworu", "maya",
		"melchor", "misato", "rei", "ritsuko", "shinji", "toji",
	}
	groups := map[string][]string{
		"magi":        {"balthasar", "melchor", "casper"},
		"pilots":      {"rei", "shinji", "asuka", "toji", "kaworu"},
		"kaji-passes": {"kaji", "kaji-security", "kaji-coverage", "kaji-refuter"},
		"all":         allRoles,
	}
	return RoleCatalogue{AllRoles: allRoles, Groups: groups}
}

// ErrUnknownRole is returned by ValidateRole for a role outside the
// catalogue. Its message matches configure.ps1's -SetModel rejection text.
type ErrUnknownRole struct {
	Role string
}

func (e *ErrUnknownRole) Error() string {
	return fmt.Sprintf("Unknown role '%s'. Valid roles: %s.", e.Role, strings.Join(Roles().AllRoles, ", "))
}

// ValidateRole reports an *ErrUnknownRole when role is not one of the 18
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
	lines := []string{"models:                             # per-role model and effort (written by tools/configure-models.ps1)"}

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
// not the model/effort it resolves to). Mirrors the raw block scan half of
// Read-NervModelsOverrides.
//
// Deviation from the port brief: the PowerShell function also cross-checks
// the raw scan against Resolve-NervModelAssignments (tools/install.ps1) and
// drops a role whose override doesn't validate there (e.g. an unresolved
// from: phase) — that cross-check needs the resolved, file-sourced
// assignment table and belongs to a package with filesystem access (P2/P3);
// ModelTable below takes the resolved phase assignments as a parameter so
// callers can apply the same display-time resolution.
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
	// Source is "default", "override", or "gentle-ai:<phase>".
	Source string
}

// PhaseAssignment is one gentle-ai claude_phase_assignments entry (model
// and effort for a named phase), used to resolve a role's from:<phase>
// override for display.
type PhaseAssignment struct {
	Model  string
	Effort string
}

// ModelTable builds the display table (role, model, effort, source) from
// the plugin default model/effort per role, the raw models: overrides, and
// gentle-ai's phase assignments (for from:<phase> display). source is
// "override" when the role has an explicit model or effort key in
// overrides, "gentle-ai:<phase>" when it only has a from key, or "default"
// when it has no override at all. A role present in overrides but not
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
// neither a known alias nor a claude-... id. Its message matches
// configure.ps1's -SetModel rejection text.
type ErrInvalidModel struct {
	Role  string
	Model string
}

func (e *ErrInvalidModel) Error() string {
	return fmt.Sprintf("Invalid model '%s' for role '%s'. Allowed: sonnet, opus, haiku, fable, inherit, a claude-... id, from:<phase>, or default.", e.Model, e.Role)
}

// ErrInvalidEffort is returned by ResolveModelSpec for an effort token
// outside the known set. Its message matches configure.ps1's -SetModel
// rejection text.
type ErrInvalidEffort struct {
	Role   string
	Effort string
}

func (e *ErrInvalidEffort) Error() string {
	return fmt.Sprintf("Invalid effort '%s' for role '%s'. Allowed: low, medium, high, xhigh, max.", e.Effort, e.Role)
}

var (
	modelAliasRe  = regexp.MustCompile(`^(sonnet|opus|haiku|fable|inherit)$`)
	customModelRe = regexp.MustCompile(`^claude-.+$`)
	fromSpecRe    = regexp.MustCompile(`(?i)^from:(.+)$`)
	effortRe      = regexp.MustCompile(`^(low|medium|high|xhigh|max)$`)
)

// ResolveModelSpec parses one -SetModel "role=<spec>" value — model[/effort]
// (a model alias — sonnet, opus, haiku, fable, inherit — or a claude-...
// id, optionally with an effort of low|medium|high|xhigh|max), from:<phase>,
// or default (case-insensitive, clears the role's override) — into the
// override to apply for role and whether it clears the override entirely.
// role is only used to format an error message. Mirrors the -SetModel
// per-entry spec parsing/validation inlined in configure.ps1's
// non-interactive body (that logic has no PowerShell function name of its
// own, so there is no case group to port from; see models_test.go).
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

	if !modelAliasRe.MatchString(modelPart) && !customModelRe.MatchString(modelPart) {
		return ModelOverride{}, false, &ErrInvalidModel{Role: role, Model: modelPart}
	}
	if effortPart != "" && !effortRe.MatchString(effortPart) {
		return ModelOverride{}, false, &ErrInvalidEffort{Role: role, Effort: effortPart}
	}

	return ModelOverride{Model: modelPart, Effort: effortPart}, false, nil
}
