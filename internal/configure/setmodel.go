package configure

import (
	"fmt"
	"sort"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/gentleai"
)

// SetModel applies one or more "role=<spec>" per-role model/effort override
// changes to the user-scope nerv.yaml's models: block, where <spec> is
// model[/effort], from:<gentle-ai-phase>, or default (clears the role's
// override). Every entry is validated (ParseSetArg, ValidateRole,
// ResolveModelSpec) before its change is applied to the in-memory override
// map; since the file is only ever written once, after every entry has been
// processed, no partial write can happen when a later entry is invalid.
// A from:<phase> missing from a readable, non-empty gentle-ai state file
// adds a warning but is still written, since the state can change later.
// A legacy role ID (config.LegacyRoleAliases) is not stale: it resolves to
// the role that replaced it, both on the command line and in the file. A
// role no longer in the catalogue is refused, except role=default when the
// file still holds an override for it, so a stale override can be cleared.
func SetModel(deps Deps, paths Paths, specs []string) (Result, error) {
	store := Store{}
	doc, exists, err := store.Load(paths.Config)
	if err != nil {
		return Result{}, err
	}
	original := []byte(doc.String())

	working := doc
	if !exists {
		working = config.Parse(missingUserConfigHeader)
	}

	overrides := config.ReadModelsOverrides(working)

	// Unreadable state means the phases are unknown: skip the check.
	phaseAssignments, err := gentleai.PhaseAssignments(paths.State)
	if err != nil {
		phaseAssignments = nil
	}

	changes := emptyChanges()
	warnings := emptyStrings()
	touched := false
	for _, raw := range specs {
		role, spec, ok := config.ParseSetArg(raw)
		if !ok {
			return Result{}, &RefusalError{Err: fmt.Errorf("Invalid --set-model entry '%s'; expected role=model[/effort], role=from:<phase>, or role=default.", raw)}
		}
		// A legacy role ID (melchor, kaji-audit) names its renamed role; the
		// overrides map is already keyed by current IDs, so the write keeps
		// one entry under the new ID.
		role = config.CanonicalRole(role)
		if err := config.ValidateRole(role); err != nil {
			// A role removed from the catalogue may still have an override in
			// the file; clearing it is the only change allowed for that role.
			if _, stale := overrides[role]; !stale || !strings.EqualFold(spec, "default") {
				return Result{}, &RefusalError{Err: err}
			}
		}

		oldDisplay := displayOverride(overrides[role])

		override, clear, err := config.ResolveModelSpec(role, spec)
		if err != nil {
			return Result{}, &RefusalError{Err: err}
		}

		var newDisplay string
		switch {
		case clear:
			delete(overrides, role)
			newDisplay = "default"
		case override.From != "":
			overrides[role] = override
			newDisplay = "from:" + override.From
			if w := missingPhaseWarning(role, override.From, paths.State, phaseAssignments); w != "" {
				warnings = append(warnings, w)
			}
		default:
			overrides[role] = override
			newDisplay = spec
		}

		if oldDisplay != newDisplay {
			changes = append(changes, Change{Key: "models." + role, From: oldDisplay, To: newDisplay})
			touched = true
		}
	}

	blockText := ""
	if len(overrides) > 0 {
		blockText = config.FormatModelsBlock(overrides)
	}
	config.SetModelsBlock(working, blockText)

	result, err := finalizeMutation(deps, paths, store, working, original, touched, changes)
	if err != nil {
		return Result{}, err
	}
	result.Warnings = append(result.Warnings, warnings...)
	return result, nil
}

// missingPhaseWarning returns the warning for a from:<phase> override whose
// phase is not a key of a known, non-empty phaseAssignments, or "" when the
// phase exists or the assignments are unknown (state absent or unreadable).
func missingPhaseWarning(role, phase, statePath string, phaseAssignments map[string]config.PhaseAssignment) string {
	if len(phaseAssignments) == 0 {
		return ""
	}
	if _, ok := phaseAssignments[phase]; ok {
		return ""
	}
	available := make([]string, 0, len(phaseAssignments))
	for key := range phaseAssignments {
		available = append(available, key)
	}
	sort.Strings(available)
	return fmt.Sprintf("models.%s: phase '%s' is not in %s claude_phase_assignments (available: %s); the role uses its plugin default until it is.",
		role, phase, statePath, strings.Join(available, ", "))
}

// displayOverride renders one role's current override for the "from ...
// to ..." change summary: "from:<phase>" when From is set,
// "<model>[/<effort>]" when Model and/or Effort is set, or "default" for
// an absent override (the zero value — ReadModelsOverrides never returns
// an entry with every field empty).
func displayOverride(o config.ModelOverride) string {
	switch {
	case o.From != "":
		return "from:" + o.From
	case o.Model != "" || o.Effort != "":
		if o.Effort != "" {
			return o.Model + "/" + o.Effort
		}
		return o.Model
	default:
		return "default"
	}
}
