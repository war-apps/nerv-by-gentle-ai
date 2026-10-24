package deployprovider_test

import (
	"io/fs"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployprovider"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

// Every deploy provider nerv ships has a valid manifest, named after its
// plugin (deploy-<name>); the GitHub host and the SSH compose target ship.
func TestEmbeddedDeployProviders_AreValid(t *testing.T) {
	exts, err := plugin.Extensions(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, ext := range exts {
		data, err := fs.ReadFile(nerv.ExtensionsFS(), ext.Name+"/"+deployprovider.ManifestFile)
		if err != nil {
			continue
		}
		m, err := deployprovider.Parse(data)
		if err != nil {
			t.Errorf("%s: %v", ext.Name, err)
			continue
		}
		if "deploy-"+m.Name != ext.Name {
			t.Errorf("%s: manifest name %q, want the plugin named deploy-<name>", ext.Name, m.Name)
		}
		if !strings.HasPrefix(ext.Name, "deploy-") || ext.ProviderKind != plugin.DeployPlugin || len(ext.Requires) != 1 || ext.Requires[0] != plugin.DeployPlugin {
			t.Errorf("%s: a deploy provider must be a deploy-<name> plugin without a task provider.yaml, requiring %s: %+v", ext.Name, plugin.DeployPlugin, ext)
		}
		kinds[m.Name] = m.Kind
	}
	if kinds["github"] != deployprovider.KindGitHost || kinds["ssh-compose"] != deployprovider.KindTarget {
		t.Errorf("embedded deploy providers %v, want github (git-host) and ssh-compose (target)", kinds)
	}
}
