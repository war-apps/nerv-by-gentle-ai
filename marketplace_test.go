package nerv

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// marketplaceEntry is one plugin of a marketplace.json, in the shape
// internal/plugin's MaterializeBundle writes.
type marketplaceEntry struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

// The repository's .claude-plugin/marketplace.json, which Claude Code reads
// when the clone itself is registered as a marketplace, lists the core
// plugin first and then every extension and provider shipped under
// plugins/, in name order, each with its ./plugins/<name> source and the
// description of its .claude-plugin/plugin.json: the same bytes nerv
// install writes into the materialized marketplace.
func TestRepoMarketplace_ListsEveryShippedPlugin(t *testing.T) {
	description := func(fsys fs.FS, path string) string {
		t.Helper()
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return m.Description
	}

	plugins := []marketplaceEntry{{Name: "nerv", Source: "./plugin", Description: description(PluginFS(), ".claude-plugin/plugin.json")}}
	entries, err := fs.ReadDir(ExtensionsFS(), ".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		plugins = append(plugins, marketplaceEntry{Name: name, Source: "./plugins/" + name, Description: description(ExtensionsFS(), name+"/.claude-plugin/plugin.json")})
	}

	want, err := json.MarshalIndent(struct {
		Name  string `json:"name"`
		Owner struct {
			Name string `json:"name"`
		} `json:"owner"`
		Plugins []marketplaceEntry `json:"plugins"`
	}{Name: "nerv", Owner: struct {
		Name string `json:"name"`
	}{Name: "Walter Rodriguez"}, Plugins: plugins}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')

	got, err := os.ReadFile(filepath.Join(".claude-plugin", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf(".claude-plugin/marketplace.json is stale; it must list the core and every folder of plugins/:\n--- got\n%s\n--- want\n%s", got, want)
	}
}
