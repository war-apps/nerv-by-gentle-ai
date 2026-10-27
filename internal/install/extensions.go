package install

import (
	"fmt"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

// selectExtensions decides which extensions and providers Install enables, in
// order: the requested ones (each after its requirements), then those
// settings already enables, then the ones the user accepts when
// deps.ConfirmExtension is set, asked in plugin.SuggestionOrder (task, its
// providers, release) and only when every requirement is already selected:
// a provider only once its extension is. An extension whose providers are bundled brings
// them all with it (plugin.Extension.Bundle), never asked.
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
	// add selects e and, for an extension that bundles its providers
	// (providers: bundled), every one of them right after it, without
	// asking.
	add := func(e plugin.Extension) {
		if has(e.Name) {
			return
		}
		selected = append(selected, e)
		for _, name := range e.Bundle {
			if p, shipped := byName[name]; shipped && !has(name) {
				if !slices.Contains(opts.Extensions, name) {
					fmt.Fprintf(deps.Stdout, "Adding %s (bundled with %s)\n", name, e.Name)
				}
				selected = append(selected, p)
			}
		}
	}

	// require adds e after its requirements, transitively (release-github
	// brings release).
	var require func(e plugin.Extension, by string) error
	require = func(e plugin.Extension, by string) error {
		for _, req := range e.Requires {
			if has(req) {
				continue
			}
			r, shipped := byName[req]
			if !shipped {
				return &RefusalError{Err: fmt.Errorf("%s requires the extension %q, which this nerv does not ship", e.Name, req)}
			}
			if err := require(r, e.Name); err != nil {
				return err
			}
		}
		if by != "" && !has(e.Name) {
			fmt.Fprintf(deps.Stdout, "Adding %s (required by %s)\n", e.Name, by)
		}
		add(e)
		return nil
	}
	for _, name := range opts.Extensions {
		e, ok := byName[name]
		if !ok {
			available := "none"
			if len(names) > 0 {
				available = strings.Join(names, ", ")
			}
			return nil, &RefusalError{Err: fmt.Errorf("unknown extension or provider %q (available: %s)", name, available)}
		}
		if err := require(e, ""); err != nil {
			return nil, err
		}
	}

	for _, e := range exts {
		if settings.PluginEnabled(e.Name + "@" + MarketplaceName) {
			add(e)
		}
	}

	if deps.ConfirmExtension == nil {
		return selected, nil
	}
	for _, e := range plugin.SuggestionOrder(exts) {
		if has(e.Name) {
			continue
		}
		if !slices.ContainsFunc(e.Requires, func(r string) bool { return !has(r) }) && deps.ConfirmExtension(e.Name, e.Description) {
			add(e)
		}
	}
	return selected, nil
}
