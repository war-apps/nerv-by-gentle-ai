package nerv

import (
	"encoding/json"
	"io/fs"
	"testing"
)

func TestPluginFS_ContainsExpectedFiles(t *testing.T) {
	pfs := PluginFS()

	requiredFiles := []string{
		".claude-plugin/plugin.json",
		"hooks/hooks.json",
		"commands/configure.md",
		"skills/nerv-orchestrator/SKILL.md",
	}
	for _, name := range requiredFiles {
		if _, err := fs.Stat(pfs, name); err != nil {
			t.Errorf("expected %q to exist in PluginFS(): %v", name, err)
		}
	}

	matches, err := fs.Glob(pfs, "agents/*.md")
	if err != nil {
		t.Fatalf("fs.Glob(agents/*.md) failed: %v", err)
	}
	if len(matches) == 0 {
		t.Error("expected at least one agents/*.md file in PluginFS()")
	}
}

func TestPluginFS_PluginJSONParsesWithVersion(t *testing.T) {
	pfs := PluginFS()

	data, err := fs.ReadFile(pfs, ".claude-plugin/plugin.json")
	if err != nil {
		t.Fatalf("reading .claude-plugin/plugin.json: %v", err)
	}

	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parsing .claude-plugin/plugin.json: %v", err)
	}
	if manifest.Version == "" {
		t.Error("expected plugin.json version to be non-empty")
	}
}
