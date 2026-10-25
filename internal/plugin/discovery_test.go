package plugin_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension/extensiontest"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

func pluginJSON(name string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(`{"name":"` + name + `","version":"1.0.0","description":"` + name + `"}`)}
}

// tree is two extensions and a provider of each: alpha (order 2) requires
// zeta (order 1), so zeta is suggested first although it sorts last.
func tree() fstest.MapFS {
	return fstest.MapFS{
		"alpha/.claude-plugin/plugin.json":     pluginJSON("alpha"),
		"alpha/extension.yaml":                 {Data: extensiontest.Extension("alpha", 2, "zeta")},
		"alpha-one/.claude-plugin/plugin.json": pluginJSON("alpha-one"),
		"alpha-one/provider.yaml":              {Data: extensiontest.Provider("alpha", "one")},
		"zeta/.claude-plugin/plugin.json":      pluginJSON("zeta"),
		"zeta/extension.yaml":                  {Data: extensiontest.Extension("zeta", 1)},
		"zeta-two/.claude-plugin/plugin.json":  pluginJSON("zeta-two"),
		"zeta-two/provider.yaml":               {Data: extensiontest.Provider("zeta", "two")},
	}
}

// S6: requirements, provider ownership and the suggestion order come from
// the extension.yaml and provider.yaml manifests, not from a fixed map.
func TestExtensions_ReadsTheManifests(t *testing.T) {
	exts, err := plugin.Extensions(tree())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]plugin.Extension{}
	for _, e := range exts {
		byName[e.Name] = e
	}
	if a := byName["alpha"]; a.Provider || a.Order != 2 || !slices.Equal(a.Requires, []string{"zeta"}) {
		t.Errorf("alpha = %+v", a)
	}
	if p := byName["alpha-one"]; !p.Provider || p.Owner != "alpha" || p.Slot != "provider" || p.Order != 2 || !slices.Equal(p.Requires, []string{"alpha"}) {
		t.Errorf("alpha-one = %+v", p)
	}
	var order []string
	for _, e := range plugin.SuggestionOrder(exts) {
		order = append(order, e.Name)
	}
	if want := []string{"zeta", "zeta-two", "alpha", "alpha-one"}; !slices.Equal(order, want) {
		t.Errorf("SuggestionOrder = %v, want %v", order, want)
	}
}

func TestExtensions_RefusesAnInvalidTree(t *testing.T) {
	tests := []struct {
		name string
		edit func(fstest.MapFS)
		want string
	}{
		{"no manifest", func(f fstest.MapFS) { delete(f, "zeta/extension.yaml") }, "zeta: neither extension.yaml nor provider.yaml"},
		{"both manifests", func(f fstest.MapFS) { f["zeta/provider.yaml"] = f["zeta-two/provider.yaml"] }, "zeta: both extension.yaml and provider.yaml"},
		{"provider of an extension not shipped", func(f fstest.MapFS) {
			delete(f, "alpha/extension.yaml")
			delete(f, "alpha/.claude-plugin/plugin.json")
		}, `alpha-one/provider.yaml: extension "alpha" is not available`},
		{"provider folder is not <extension>-<name>", func(f fstest.MapFS) {
			f["zeta-box/.claude-plugin/plugin.json"] = pluginJSON("zeta-box")
			f["zeta-box/provider.yaml"] = f["zeta-two/provider.yaml"]
		}, "a provider of extension zeta named two lives in the folder zeta-two"},
		{"provider misfits its slot", func(f fstest.MapFS) {
			f["zeta-two/provider.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(string(extensiontest.Provider("zeta", "two")), "slot: provider", "slot: crate", 1))}
		}, `slot "crate" is not a slot of extension zeta`},
		{"extension name is not its folder", func(f fstest.MapFS) {
			f["zeta/extension.yaml"] = &fstest.MapFile{Data: extensiontest.Extension("zed", 1)}
		}, `name "zed" must match its folder "zeta"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tree()
			tt.edit(f)
			_, err := plugin.Extensions(f)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Extensions: %v, want %q", err, tt.want)
			}
		})
	}
}
