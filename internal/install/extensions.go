package install

import (
	"fmt"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

// selectExtensions decides which extension plugins Install enables, in
// order: the requested ones (each after its requirements), then those
// settings already enables, then the ones the user accepts when
// deps.ConfirmExtension is set (the task plugin first, then each provider
// whose requirements are selected).
func selectExtensions(deps Deps, opts Options, settings *claude.Settings) ([]plugin.Extension, error) {
	exts, err := plugin.Extensions(deps.ExtensionsFS)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]plugin.Extension, len(exts))
	names := make([]string, 0, len(exts))
	for _, e := range exts {
		byName[e.Name] = e
		names = append(names, e.Name)
	}

	var selected []plugin.Extension
	has := func(name string) bool {
		return slices.ContainsFunc(selected, func(e plugin.Extension) bool { return e.Name == name })
	}
	add := func(e plugin.Extension) {
		if !has(e.Name) {
			selected = append(selected, e)
		}
	}

	for _, name := range opts.Extensions {
		e, ok := byName[name]
		if !ok {
			available := "none"
			if len(names) > 0 {
				available = strings.Join(names, ", ")
			}
			return nil, &RefusalError{Err: fmt.Errorf("unknown extension plugin %q (available: %s)", name, available)}
		}
		for _, req := range e.Requires {
			if has(req) {
				continue
			}
			r, shipped := byName[req]
			if !shipped {
				return nil, &RefusalError{Err: fmt.Errorf("extension plugin %q requires %q, which this nerv does not ship", e.Name, req)}
			}
			fmt.Fprintf(deps.Stdout, "Adding %s (required by %s)\n", req, e.Name)
			add(r)
		}
		add(e)
	}

	for _, e := range exts {
		if settings.PluginEnabled(e.Name + "@" + marketplaceName) {
			add(e)
		}
	}

	if deps.ConfirmExtension == nil {
		return selected, nil
	}
	for _, providers := range []bool{false, true} {
		for _, e := range exts {
			if e.Provider != providers || has(e.Name) {
				continue
			}
			if !slices.ContainsFunc(e.Requires, func(r string) bool { return !has(r) }) && deps.ConfirmExtension(e.Name, e.Description) {
				add(e)
			}
		}
	}
	return selected, nil
}
