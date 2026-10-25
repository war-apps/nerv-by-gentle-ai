package taskprovider_test

import (
	"io/fs"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
	"github.com/war-apps/nerv-by-gentle-ai/internal/taskprovider"
)

// Every provider nerv ships must satisfy the contract: a valid manifest
// whose operation files exist inside its plugin.
func TestEmbeddedProviders_AreValid(t *testing.T) {
	exts, err := plugin.Extensions(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	providers := 0
	for _, ext := range exts {
		if ext.Owner != plugin.TaskPlugin {
			continue
		}
		providers++
		root, err := fs.Sub(nerv.ExtensionsFS(), ext.Name)
		if err != nil {
			t.Fatal(err)
		}
		m, err := taskprovider.Load(nerv.ExtensionsFS(), ext.Name)
		if err != nil {
			t.Errorf("%s: %v", ext.Name, err)
			continue
		}
		if "task-"+m.Name != ext.Name {
			t.Errorf("%s: manifest name %q, want the plugin named task-<name>", ext.Name, m.Name)
		}
		if err := m.CheckFiles(root); err != nil {
			t.Errorf("%s: %v", ext.Name, err)
		}
	}
	if providers == 0 {
		t.Fatal("no embedded provider: the Teamwork provider must ship")
	}
}
