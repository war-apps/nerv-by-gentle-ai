package nerv

import (
	"io/fs"
	"testing"
)

// /task:setup configures a repository's tasks: section on its own, without
// nerv configure's wizard: the provider, its repo-scope keys from its
// provider.yaml, and a connection check pointing at the provider's README.
func TestTaskSetupCommand_ConfiguresTheTasksSectionOnly(t *testing.T) {
	data, err := fs.ReadFile(ExtensionsFS(), "task/commands/setup.md")
	if err != nil {
		t.Fatalf("plugins/task/commands/setup.md is missing: %v", err)
	}
	assertContainsAll(t, "task/commands/setup.md", string(data),
		"description:", "Load the `task` skill first",
		"`tasks:`", ".ai-workflow/config.yaml", "provider.yaml", "`scope: repo`", "`scope: user`",
		"defaults.tasks", "installed_plugins.json", ".ai-workflow/providers/task-",
		"tool check", "README.md", "one question at a time", "explicit yes",
		"Never commit")
}
