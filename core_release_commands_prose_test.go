package nerv

import (
	"io/fs"
	"strings"
	"testing"
)

// removedCoreCommands are the release commands that left core (releasing
// belongs to the nerv-release tool now).
var removedCoreCommands = []string{"/nerv:ship", "/nerv:version", "/nerv:release-setup", "/nerv:pipeline"}

// Core nerv ships no release command and never names one: releasing,
// versioning and deploying belong to the nerv-release tool.
func TestCorePlugin_HasNoReleaseCommands(t *testing.T) {
	for _, name := range []string{"release", "version", "release-setup", "pipeline"} {
		if _, err := fs.Stat(PluginFS(), "commands/"+name+".md"); err == nil {
			t.Errorf("plugin/commands/%s.md still exists; it left core for the nerv-release tool", name)
		}
	}
	err := fs.WalkDir(PluginFS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(PluginFS(), path)
		if err != nil {
			return err
		}
		for _, cmd := range removedCoreCommands {
			if strings.Contains(string(data), cmd) {
				t.Errorf("plugin/%s still names %s", path, cmd)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
