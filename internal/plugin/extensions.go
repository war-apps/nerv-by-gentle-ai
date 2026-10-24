package plugin

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/taskprovider"
)

// TaskPlugin is the extension every task provider needs installed.
const TaskPlugin = "task"

// DeployPlugin is the extension every deploy provider needs installed.
const DeployPlugin = "deploy"

// ShipPlugin is the extension DeployPlugin needs installed: deploy consumes
// the artifacts ship leaves.
const ShipPlugin = "ship"

// pluginRequires are the requirements of the extension plugins that are not
// providers (a provider requires its kind's plugin): a fixed map, because a
// plugin.json cannot carry its own key for them.
var pluginRequires = map[string][]string{
	DeployPlugin: {ShipPlugin},
}

// suggestionOrder is the order nerv install suggests the plugin families
// in: each plugin, then its providers. A plugin outside it comes after, in
// name order.
var suggestionOrder = []string{TaskPlugin, ShipPlugin, DeployPlugin}

// deployProviderManifest marks a deploy provider plugin (see
// internal/deployprovider.ManifestFile; not imported, to keep this package
// free of the deploy contract).
const deployProviderManifest = "deploy-provider.yaml"

// Extension is an optional plugin shipped next to the core nerv plugin in
// the same marketplace, installed as <Name>@nerv.
type Extension struct {
	Name        string
	Version     string
	Description string
	// Provider is true for a task provider (a plugin with a provider.yaml
	// at its root) or a deploy provider (a deploy-provider.yaml).
	Provider bool
	// ProviderKind is "task" or "deploy" for a provider, empty otherwise.
	ProviderKind string
	// Requires lists the extensions that must be installed with this one.
	Requires []string
}

// Extensions lists the extension plugins of fsys, one per top-level
// folder holding a .claude-plugin/plugin.json whose name is the folder's
// name, in name order. A nil fsys has none.
func Extensions(fsys fs.FS) ([]Extension, error) {
	if fsys == nil {
		return nil, nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("listing extension plugins: %w", err)
	}

	var exts []Extension
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

		ext := Extension{Name: manifest.Name, Version: manifest.Version, Description: manifest.Description,
			Requires: append([]string(nil), pluginRequires[manifest.Name]...)}
		if _, err := fs.Stat(fsys, e.Name()+"/"+taskprovider.ManifestFile); err == nil {
			ext.Provider, ext.ProviderKind = true, TaskPlugin
			ext.Requires = []string{TaskPlugin}
		} else if _, err := fs.Stat(fsys, e.Name()+"/"+deployProviderManifest); err == nil {
			ext.Provider, ext.ProviderKind = true, DeployPlugin
			ext.Requires = []string{DeployPlugin}
		}
		exts = append(exts, ext)
	}
	sort.Slice(exts, func(i, j int) bool { return exts[i].Name < exts[j].Name })
	return exts, nil
}

// SuggestionOrder returns exts in the order nerv install suggests them:
// task, then the task providers, then ship, then deploy, then the deploy
// providers (each family's plugin before its providers); any other
// extension after them, in name order. Asking only for an extension whose
// requirements are selected then skips deploy when ship is declined and the
// providers of a declined plugin.
func SuggestionOrder(exts []Extension) []Extension {
	rank := func(e Extension) (int, int) {
		family := e.Name
		if e.Provider {
			family = e.ProviderKind
		}
		i := slices.Index(suggestionOrder, family)
		if i < 0 {
			i = len(suggestionOrder)
		}
		provider := 0
		if e.Provider {
			provider = 1
		}
		return i, provider
	}
	out := slices.Clone(exts)
	slices.SortStableFunc(out, func(a, b Extension) int {
		fa, pa := rank(a)
		fb, pb := rank(b)
		if c := cmp.Compare(fa, fb); c != 0 {
			return c
		}
		if c := cmp.Compare(pa, pb); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}
