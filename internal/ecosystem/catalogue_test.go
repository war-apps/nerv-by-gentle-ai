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
		commands string
		replaces []string
	}{
		{"nerv-task", tools[0], "war-apps/nerv-task-by-gentle-ai", "/task:*", []string{"task", "task-teamwork"}},
		{"nerv-release", tools[1], "war-apps/nerv-release-by-gentle-ai", "/release:*", []string{
			"release", "release-bash", "release-conventional-commits", "release-docker-compose",
			"release-github", "release-script", "release-ssh-compose",
			"deploy", "deploy-bash", "deploy-github", "deploy-ssh-compose",
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
			if tt.got.Commands != tt.commands {
				t.Errorf("Commands = %q, want %q", tt.got.Commands, tt.commands)
			}
			if !slices.Equal(tt.got.Replaces, tt.replaces) {
				t.Errorf("Replaces = %v, want %v", tt.got.Replaces, tt.replaces)
			}
		})
	}
}
