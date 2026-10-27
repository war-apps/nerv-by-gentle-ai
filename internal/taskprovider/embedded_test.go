package taskprovider_test

import (
	"encoding/json"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

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

// mcpToolName matches a tool of Teamwork's official MCP server named in a
// procedure (twprojects-<operation>).
var mcpToolName = regexp.MustCompile("`(twprojects-[a-z_]+)`")

// The Teamwork provider calls Teamwork's official MCP server: every tool a
// procedure names is declared in mcp.tools, every declared tool is used,
// and no procedure names a tool of the old local server (teamwork_*).
func TestTeamworkProvider_ProceduresUseTheDeclaredOfficialTools(t *testing.T) {
	m, err := taskprovider.Load(nerv.ExtensionsFS(), "task-teamwork")
	if err != nil {
		t.Fatal(err)
	}
	if m.MCP.Server != "teamwork" {
		t.Errorf("mcp.server = %q, want teamwork", m.MCP.Server)
	}
	used := map[string]bool{}
	for op, file := range m.Operations {
		data, err := fs.ReadFile(nerv.ExtensionsFS(), "task-teamwork/"+file)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		if strings.Contains(string(data), "teamwork_") {
			t.Errorf("%s names a tool of the old local server (teamwork_*)", file)
		}
		for _, mt := range mcpToolName.FindAllStringSubmatch(string(data), -1) {
			used[mt[1]] = true
			if !slices.Contains(m.MCP.Tools, mt[1]) {
				t.Errorf("%s calls %s, which mcp.tools does not declare", file, mt[1])
			}
		}
	}
	for _, tool := range m.MCP.Tools {
		if !used[tool] {
			t.Errorf("mcp.tools declares %s, which no procedure calls", tool)
		}
	}
	// The plugin ships the server it declares, with no credential: users
	// sign in with OAuth.
	data, err := fs.ReadFile(nerv.ExtensionsFS(), "task-teamwork/.mcp.json")
	if err != nil {
		t.Fatalf("task-teamwork ships no .mcp.json: %v", err)
	}
	var servers map[string]map[string]any
	if err := json.Unmarshal(data, &servers); err != nil {
		t.Fatalf(".mcp.json: %v", err)
	}
	s, ok := servers[m.MCP.Server]
	if !ok || len(servers) != 1 || s["type"] != "http" || s["url"] != "https://mcp.ai.teamwork.com" {
		t.Errorf(".mcp.json = %s, want only %s, http, https://mcp.ai.teamwork.com", data, m.MCP.Server)
	}
	if _, ok := s["headers"]; ok {
		t.Errorf(".mcp.json carries headers; the server is reached with OAuth, no credential in the plugin")
	}
}

// copyTree copies fsys into a MapFS a test can edit.
func copyTree(t *testing.T, fsys fs.FS) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		out[path] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// L11: the task rule "an event cannot need a not_implemented operation" is
// declared in plugins/task/extension.yaml, so discovery, which knows no
// extension by name, enforces it for every task provider.
func TestDiscovery_EnforcesTheTaskEventRule(t *testing.T) {
	tree := copyTree(t, nerv.ExtensionsFS())
	const file = "task-teamwork/provider.yaml"
	text := string(tree[file].Data)
	if !strings.Contains(text, "  comment: procedures/comment.md\n") || !strings.Contains(text, "  - run.halted\n") {
		t.Fatalf("%s no longer implements comment or handles run.halted", file)
	}
	tree[file] = &fstest.MapFile{Data: []byte(strings.Replace(text, "  comment: procedures/comment.md\n", "  comment: not_implemented\n", 1))}
	_, err := plugin.Extensions(tree)
	if err == nil || !strings.Contains(err.Error(), "task-teamwork/provider.yaml: events: audit.ranked needs comment, declared not_implemented") {
		t.Errorf("Extensions: %v, want the event rule refused", err)
	}
}

// The events the task extension hands its providers, and the operations
// each needs, are the contract's Events table.
func TestTaskExtension_SlotEventsAreTheContractEvents(t *testing.T) {
	slot, ok := taskExtension(t).Slot(taskprovider.Slot)
	if !ok {
		t.Fatal("no provider slot")
	}
	if !maps.EqualFunc(slot.Events, taskprovider.Events, slices.Equal) {
		t.Errorf("slot events %v, want %v", slot.Events, taskprovider.Events)
	}
}
