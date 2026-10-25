package deployprovider_test

import (
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployprovider"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

// Every deploy provider nerv ships has a valid provider.yaml, in a
// deploy-<name> plugin requiring deploy: the GitHub host, and the SSH
// compose and bash targets.
func TestEmbeddedDeployProviders_AreValid(t *testing.T) {
	exts, err := plugin.Extensions(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	slots := map[string]string{}
	for _, ext := range exts {
		if ext.Owner != plugin.DeployPlugin {
			continue
		}
		m, err := deployprovider.Load(nerv.ExtensionsFS(), ext.Name)
		if err != nil {
			t.Errorf("%s: %v", ext.Name, err)
			continue
		}
		if "deploy-"+m.Name != ext.Name || len(ext.Requires) != 1 || ext.Requires[0] != plugin.DeployPlugin || ext.Slot != m.Slot {
			t.Errorf("%s: a deploy provider must be a deploy-<name> plugin requiring %s: %+v", ext.Name, plugin.DeployPlugin, ext)
		}
		slots[m.Name] = m.Slot
	}
	if slots["github"] != deployprovider.SlotHost || slots["ssh-compose"] != deployprovider.SlotTarget || slots["bash"] != deployprovider.SlotTarget {
		t.Errorf("embedded deploy providers %v, want github (host), ssh-compose and bash (targets)", slots)
	}
}
