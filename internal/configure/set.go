package configure

import (
	"fmt"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// Set applies one or more "key=value" managed-key changes to the user-scope
// nerv.yaml: every entry is parsed and validated (ParseSetArg,
// ValidateManagedKey, ValidateManagedValue) before anything is written —
// the first invalid entry refuses the whole batch and touches nothing.
func Set(deps Deps, paths Paths, args []string) (Result, error) {
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

	type entry struct{ Key, Value string }
	entries := make([]entry, 0, len(args))
	for _, raw := range args {
		key, value, ok := config.ParseSetArg(raw)
		if !ok {
			return Result{}, &RefusalError{Err: fmt.Errorf("Invalid --set entry '%s'; expected key=value.", raw)}
		}
		if err := config.ValidateManagedKey(key); err != nil {
			return Result{}, &RefusalError{Err: err}
		}
		if err := config.ValidateManagedValue(key, value); err != nil {
			return Result{}, &RefusalError{Err: err}
		}
		entries = append(entries, entry{Key: key, Value: value})
	}

	changes := emptyChanges()
	touched := false
	for _, e := range entries {
		oldValue, _ := config.ManagedValue(working, e.Key)
		changed, err := config.SetManagedValue(working, e.Key, e.Value)
		if err != nil {
			// Already validated above; unreachable in practice.
			return Result{}, &RefusalError{Err: err}
		}
		if changed {
			changes = append(changes, Change{Key: e.Key, From: oldValue, To: e.Value})
			touched = true
		}
	}

	return finalizeMutation(deps, paths, store, working, original, touched, changes)
}

// finalizeMutation writes working to paths.Config through store when
// touched is true and its text actually differs from original, and
// assembles the shared Result shape. Shared by Set and SetModel — the only
// two modes that mutate the user-scope nerv.yaml.
func finalizeMutation(deps Deps, paths Paths, store Store, working *config.Document, original []byte, touched bool, changes []Change) (Result, error) {
	result := Result{
		Changed:    false,
		Changes:    changes,
		Written:    emptyStrings(),
		Warnings:   emptyStrings(),
		ConfigPath: paths.Config,
		Backup:     nil,
	}

	if touched && working.String() != string(original) {
		written, backup, removed, err := store.Save(paths.Config, working, original, deps.Now())
		if err != nil {
			return Result{}, err
		}
		if written {
			result.Changed = true
			result.Written = []string{paths.Config}
			result.Removed = removed
			if backup != "" {
				b := backup
				result.Backup = &b
			}
		}
	}

	return result, nil
}
