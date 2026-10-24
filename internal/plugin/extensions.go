package plugin

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"

	"github.com/war-apps/nerv-by-gentle-ai/internal/taskprovider"
)

// TaskPlugin is the extension every task provider needs installed.
const TaskPlugin = "task"

// DeployPlugin is the extension every deploy provider needs installed.
const DeployPlugin = "deploy"

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

		ext := Extension{Name: manifest.Name, Version: manifest.Version, Description: manifest.Description}
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
