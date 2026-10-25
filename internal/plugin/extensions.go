package plugin

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
)

// The extensions nerv ships, by name. Their requirements, providers and
// suggestion order come from their extension.yaml manifests.
const (
	// TaskPlugin is the task extension, whose providers are task trackers.
	TaskPlugin = "task"
	// ReleasePlugin is the release extension.
	ReleasePlugin = "release"
	// DeployPlugin is the deploy extension, which requires release.
	DeployPlugin = "deploy"
)

// Extension is an optional plugin shipped next to the core nerv plugin in
// the same marketplace, installed as <Name>@nerv: an extension (an
// extension.yaml at its root) or one of an extension's providers (a
// provider.yaml).
type Extension struct {
	Name        string
	Version     string
	Description string
	// Provider is true for a provider plugin.
	Provider bool
	// Owner is the extension a provider belongs to (its provider.yaml
	// extension), and Slot the slot it fills; both empty for an extension.
	Owner string
	Slot  string
	// Requires lists the extensions that must be installed with this one:
	// an extension's extension.yaml requires, a provider's own extension.
	Requires []string
	// Order is the extension's suggest_order (its owner's for a provider);
	// 0 is unset.
	Order int
	// Bundle is, for an extension whose extension.yaml says providers:
	// bundled, every provider fsys ships for it, by plugin name in name
	// order: they are installed with the extension, without being asked.
	// Bundled marks such a provider.
	Bundle  []string
	Bundled bool
}

// Extensions lists the extensions and providers of fsys, one per top-level
// folder holding a .claude-plugin/plugin.json whose name is the folder's
// name, in name order. Each folder holds exactly one manifest, checked
// whole: an extension.yaml, or a provider.yaml in a folder named
// <extension>-<provider> whose extension fsys also ships and whose slot it
// fits. Every requires must name an extension fsys ships, with no cycle.
// An error names the offending manifest by its path in fsys and the rule
// it breaks; a nil fsys has none.
func Extensions(fsys fs.FS) ([]Extension, error) {
	if fsys == nil {
		return nil, nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("listing extensions and providers: %w", err)
	}

	var exts []Extension
	bundles := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := e.Name() + "/" + pluginManifestPath
		data, err := fs.ReadFile(fsys, manifestPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", manifestPath, err)
		}
		var manifest struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", manifestPath, err)
		}
		if manifest.Name != e.Name() {
			return nil, fmt.Errorf("%s: name %q must match its folder %q", manifestPath, manifest.Name, e.Name())
		}
		ext := Extension{Name: manifest.Name, Version: manifest.Version, Description: manifest.Description}
		bundles[ext.Name], err = describe(fsys, &ext)
		if err != nil {
			return nil, err
		}
		exts = append(exts, ext)
	}
	sort.Slice(exts, func(i, j int) bool { return exts[i].Name < exts[j].Name })
	if err := checkRequires(exts); err != nil {
		return nil, err
	}
	markBundles(exts, bundles)
	return exts, nil
}

// checkRequires refuses a tree whose extensions require an extension it
// does not ship (a provider is not one) or require each other in a cycle,
// so nerv install never meets either.
func checkRequires(exts []Extension) error {
	requires := map[string][]string{}
	for _, e := range exts {
		if !e.Provider {
			requires[e.Name] = e.Requires
		}
	}
	isExtension := func(name string) bool {
		_, ok := requires[name]
		return ok
	}
	if problems := extension.RequiresProblems(requires, isExtension); len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// markBundles fills Bundle and Bundled: an extension in bundles gets
// every provider of its own (exts is in name order).
func markBundles(exts []Extension, bundles map[string]bool) {
	for i := range exts {
		e := &exts[i]
		if e.Provider || !bundles[e.Name] {
			continue
		}
		for j := range exts {
			if p := &exts[j]; p.Provider && p.Owner == e.Name {
				e.Bundle = append(e.Bundle, p.Name)
				p.Bundled = true
			}
		}
	}
}

// describe fills ext from the manifest in its folder and reports whether
// an extension bundles its providers.
func describe(fsys fs.FS, ext *Extension) (bool, error) {
	has := func(file string) bool {
		_, err := fs.Stat(fsys, ext.Name+"/"+file)
		return err == nil
	}
	isExt, isProvider := has(extension.ManifestFile), has(extension.ProviderFile)
	switch {
	case isExt && isProvider:
		return false, fmt.Errorf("%s: both %s and %s: a plugin is an extension or a provider (docs/extensions.md)", ext.Name, extension.ManifestFile, extension.ProviderFile)
	case isExt:
		m, err := extension.LoadManifest(fsys, ext.Name)
		if err != nil {
			return false, err
		}
		ext.Requires, ext.Order = slices.Clone(m.Requires), m.SuggestOrder
		return m.Bundles(), nil
	case isProvider:
		p, owner, err := extension.LoadProvider(fsys, ext.Name)
		if err != nil {
			return false, err
		}
		ext.Provider, ext.Owner, ext.Slot = true, owner.Name, p.Slot
		ext.Requires, ext.Order = []string{owner.Name}, owner.SuggestOrder
	default:
		return false, fmt.Errorf("%s: neither %s nor %s: every extension and provider speaks the extension contract (docs/extensions.md)", ext.Name, extension.ManifestFile, extension.ProviderFile)
	}
	return false, nil
}

// SuggestionOrder returns exts in the order nerv install suggests them:
// by their extension's suggest_order (task, release, deploy), each
// extension followed by its providers in name order; extensions without a
// suggest_order come after them, by name. Asking only for an extension
// whose requirements are selected then skips deploy when release is
// declined and the providers of a declined extension.
func SuggestionOrder(exts []Extension) []Extension {
	family := func(e Extension) string {
		if e.Provider {
			return e.Owner
		}
		return e.Name
	}
	order := func(e Extension) int {
		if e.Order <= 0 {
			return math.MaxInt
		}
		return e.Order
	}
	out := slices.Clone(exts)
	slices.SortStableFunc(out, func(a, b Extension) int {
		if c := cmp.Compare(order(a), order(b)); c != 0 {
			return c
		}
		if c := strings.Compare(family(a), family(b)); c != 0 {
			return c
		}
		if a.Provider != b.Provider {
			if a.Provider {
				return 1
			}
			return -1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}
