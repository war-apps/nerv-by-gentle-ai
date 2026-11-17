package ecosystem

import (
	"slices"
	"testing"
)

func TestCatalogue_ListsNervTask(t *testing.T) {
	tools := Catalogue()
	if len(tools) != 1 {
		t.Fatalf("Catalogue() = %d tools, want 1 (nerv-task only; nerv-release joins in P4)", len(tools))
	}
	got := tools[0]
	if got.Name != "nerv-task" {
		t.Errorf("Name = %q, want nerv-task", got.Name)
	}
	if got.Repo != "war-apps/nerv-task-by-gentle-ai" {
		t.Errorf("Repo = %q, want war-apps/nerv-task-by-gentle-ai", got.Repo)
	}
	if got.Binary != "nerv-task" {
		t.Errorf("Binary = %q, want nerv-task", got.Binary)
	}
	if got.Description == "" {
		t.Error("Description is empty")
	}
	if !slices.Equal(got.Replaces, []string{"task", "task-teamwork"}) {
		t.Errorf("Replaces = %v, want [task task-teamwork]", got.Replaces)
	}
}

func TestReplacedExtensions_UnionsOnlyInstalledTools(t *testing.T) {
	tools := []Tool{
		{Name: "nerv-task", Replaces: []string{"task", "task-teamwork"}},
		{Name: "nerv-release", Replaces: []string{"release"}},
	}

	got := ReplacedExtensions(tools, map[string]bool{"nerv-task": true})
	if !slices.Equal(got, []string{"task", "task-teamwork"}) {
		t.Errorf("ReplacedExtensions = %v, want [task task-teamwork]", got)
	}
}

func TestReplacedExtensions_NoneInstalledIsEmpty(t *testing.T) {
	got := ReplacedExtensions(Catalogue(), map[string]bool{})
	if len(got) != 0 {
		t.Errorf("ReplacedExtensions = %v, want empty", got)
	}
}

func TestReplacedExtensions_DeduplicatesAcrossTools(t *testing.T) {
	tools := []Tool{
		{Name: "a", Replaces: []string{"task"}},
		{Name: "b", Replaces: []string{"task", "release"}},
	}
	got := ReplacedExtensions(tools, map[string]bool{"a": true, "b": true})
	if !slices.Equal(got, []string{"task", "release"}) {
		t.Errorf("ReplacedExtensions = %v, want [task release] (deduplicated, first-seen order)", got)
	}
}

func TestReplacingTool_FindsByReplacedExtensionName(t *testing.T) {
	tools := []Tool{{Name: "nerv-task", Replaces: []string{"task", "task-teamwork"}}}
	got, ok := ReplacingTool(tools, "task-teamwork")
	if !ok || got.Name != "nerv-task" {
		t.Errorf("ReplacingTool(tools, %q) = %v, %v, want nerv-task, true", "task-teamwork", got, ok)
	}
	if _, ok := ReplacingTool(tools, "release"); ok {
		t.Error(`ReplacingTool(tools, "release") = true, want false (release is not replaced)`)
	}
}
