package ecosystem

import (
	"slices"
	"testing"
)

func TestCatalogue_ListsNervTaskThenNervRelease(t *testing.T) {
	tools := Catalogue()
	if len(tools) != 2 {
		t.Fatalf("Catalogue() = %d tools, want 2 (nerv-task, nerv-release)", len(tools))
	}
	tests := []struct {
		name     string
		got      Tool
		repo     string
		replaces []string
	}{
		{"nerv-task", tools[0], "war-apps/nerv-task-by-gentle-ai", []string{"task", "task-teamwork"}},
		{"nerv-release", tools[1], "war-apps/nerv-release-by-gentle-ai", []string{
			"release", "release-bash", "release-conventional-commits", "release-docker-compose",
			"release-github", "release-script", "release-ssh-compose",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.Name != tt.name {
				t.Errorf("Name = %q, want %q", tt.got.Name, tt.name)
			}
			if tt.got.Repo != tt.repo {
				t.Errorf("Repo = %q, want %q", tt.got.Repo, tt.repo)
			}
			if tt.got.Binary != tt.name {
				t.Errorf("Binary = %q, want %q", tt.got.Binary, tt.name)
			}
			if tt.got.Description == "" {
				t.Error("Description is empty")
			}
			if !slices.Equal(tt.got.Replaces, tt.replaces) {
				t.Errorf("Replaces = %v, want %v", tt.got.Replaces, tt.replaces)
			}
		})
	}
}

func TestReplacingTool_CatalogueNamesNervReleaseForEveryReleaseExtension(t *testing.T) {
	for _, ext := range []string{
		"release", "release-bash", "release-conventional-commits", "release-docker-compose",
		"release-github", "release-script", "release-ssh-compose",
	} {
		got, ok := ReplacingTool(Catalogue(), ext)
		if !ok || got.Name != "nerv-release" {
			t.Errorf("ReplacingTool(Catalogue(), %q) = %q, %v, want nerv-release, true", ext, got.Name, ok)
		}
	}
	if got, ok := ReplacingTool(Catalogue(), "task-teamwork"); !ok || got.Name != "nerv-task" {
		t.Errorf("ReplacingTool(Catalogue(), task-teamwork) = %q, %v, want nerv-task, true", got.Name, ok)
	}
}

func TestReplacedExtensions_CatalogueOnlyNervReleaseInstalled(t *testing.T) {
	got := ReplacedExtensions(Catalogue(), map[string]bool{"nerv-release": true})
	if slices.Contains(got, "task") || len(got) != 7 || got[0] != "release" {
		t.Errorf("ReplacedExtensions = %v, want the 7 release extensions only", got)
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
