package configure

import (
	"fmt"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
)

// SetModel applies one or more "role=<spec>" per-role model/effort override
// changes to the user-scope nerv.yaml's models: block, where <spec> is
// model[/effort], from:<gentle-ai-phase>, or default (clears the role's
// override). Every entry is validated (ParseSetArg, ValidateRole,
// ResolveModelSpec) before its change is applied to the in-memory override
// map; since the file is only ever written once, after every entry has been
// processed, no partial write can happen when a later entry is invalid.
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

	changes := emptyChanges()
	touched := false
	for _, raw := range specs {
		role, spec, ok := config.ParseSetArg(raw)
		if !ok {
			return Result{}, &RefusalError{Err: fmt.Errorf("Invalid --set-model entry '%s'; expected role=model[/effort], role=from:<phase>, or role=default.", raw)}
		}
		if err := config.ValidateRole(role); err != nil {
			return Result{}, &RefusalError{Err: err}
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

	return finalizeMutation(deps, paths, store, working, original, touched, changes)
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
