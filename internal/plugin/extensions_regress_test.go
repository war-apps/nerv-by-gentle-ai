package plugin_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

func TestExtensions_EmbeddedTaskTeamworkIsAProviderRequiringTask(t *testing.T) {
	exts, err := plugin.Extensions(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(exts, func(e plugin.Extension) bool { return e.Name == "task-teamwork" })
	if i < 0 {
		t.Fatalf("task-teamwork not embedded: %+v", exts)
	}
	if !exts[i].Provider || !slices.Equal(exts[i].Requires, []string{plugin.TaskPlugin}) {
		t.Errorf("task-teamwork = %+v, want Provider and Requires [task]", exts[i])
	}
}

func TestMaterializeBundle_RemovesADroppedExtensionFolder(t *testing.T) {
	dest := t.TempDir()
	manifest := func(n string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(`{"name":"` + n + `","version":"1.0.0","description":"d"}`)}
	}
	v1 := fstest.MapFS{
		"task/.claude-plugin/plugin.json": manifest("task"),
		"gone/.claude-plugin/plugin.json": manifest("gone"),
	}
	if _, err := plugin.MaterializeBundle(nerv.PluginFS(), v1, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := plugin.MaterializeBundle(nerv.PluginFS(), fstest.MapFS{"task/.claude-plugin/plugin.json": manifest("task")}, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "plugins", "gone")); !os.IsNotExist(err) {
		t.Errorf("plugins/gone survived re-materialize (stat err = %v)", err)
	}
}
