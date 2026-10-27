package deployprovider_test

import (
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployprovider"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

// Every deploy provider nerv ships has a valid provider.yaml, in a
// release-<name> plugin requiring release: the GitHub host, and the SSH
// compose and script targets. The release extension's version and build
// providers are not deploy providers.
func TestEmbeddedDeployProviders_AreValid(t *testing.T) {
	exts, err := plugin.Extensions(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	slots := map[string]string{}
	for _, ext := range exts {
		if ext.Owner != plugin.ReleasePlugin || (ext.Slot != deployprovider.SlotHost && ext.Slot != deployprovider.SlotTarget) {
			if ext.Provider && ext.Owner == plugin.ReleasePlugin {
				if _, err := deployprovider.Load(nerv.ExtensionsFS(), ext.Name); err == nil {
					t.Errorf("%s: a %s provider loads as a deploy provider", ext.Name, ext.Slot)
				}
			}
			continue
		}
		m, err := deployprovider.Load(nerv.ExtensionsFS(), ext.Name)
		if err != nil {
			t.Errorf("%s: %v", ext.Name, err)
			continue
		}
		if "release-"+m.Name != ext.Name || len(ext.Requires) != 1 || ext.Requires[0] != plugin.ReleasePlugin || ext.Slot != m.Slot {
			t.Errorf("%s: a deploy provider must be a release-<name> plugin requiring %s: %+v", ext.Name, plugin.ReleasePlugin, ext)
		}
		slots[m.Name] = m.Slot
	}
	if slots["github"] != deployprovider.SlotHost || slots["ssh-compose"] != deployprovider.SlotTarget || slots["script"] != deployprovider.SlotTarget || len(slots) != 3 {
		t.Errorf("embedded deploy providers %v, want github (host), ssh-compose and script (targets)", slots)
	}
}
