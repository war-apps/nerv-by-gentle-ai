package nerv

import (
	"io/fs"
	"strings"
	"testing"
)

// /nerv:init ends by offering every setup flow that exists on the machine:
// NERV's own configuration and the repository setup of the nerv-task and
// nerv-release tools, each run by following its command file or running
// the tool's command; the removed task and release extensions are never
// named as setup flows.
func TestInitCommand_OffersSetupFlows(t *testing.T) {
	data, err := fs.ReadFile(PluginFS(), "commands/init.md")
	if err != nil {
		t.Fatalf("reading plugin/commands/init.md: %v", err)
	}
	assertContainsAll(t, "commands/init.md", string(data),
		"/nerv:configure", "nerv-task init", "nerv-release init",
		"multi-select", "Follow the", "not offered", "nerv install --tools <tool>")
	for _, gone := range []string{"task extension", "release extension", "defaults.tasks"} {
		if strings.Contains(string(data), gone) {
			t.Errorf("commands/init.md still names %q", gone)
		}
	}
}

// S11/U4: once nerv-task is installed, /nerv:init offers its own
// repository setup ("nerv-task init"), and the setup-flow prompt always
// carries an explicit "None" choice.
func TestInitCommand_OffersNervTaskInitInsteadOfTaskSetupWhenInstalled(t *testing.T) {
	data, err := fs.ReadFile(PluginFS(), "commands/init.md")
	if err != nil {
		t.Fatalf("reading plugin/commands/init.md: %v", err)
	}
	assertContainsAll(t, "commands/init.md", string(data),
		"nerv-task init", "nerv-task is installed", "None")
}

// S1 (nerv-clean): once nerv-release is installed, /nerv:init offers its
// own repository setup ("nerv-release init"), not offered when it is not
// installed, and the P4 placeholder is gone.
func TestInitCommand_OffersNervReleaseInitInsteadOfReleaseSetupWhenInstalled(t *testing.T) {
	data, err := fs.ReadFile(PluginFS(), "commands/init.md")
	if err != nil {
		t.Fatalf("reading plugin/commands/init.md: %v", err)
	}
	text := string(data)
	assertContainsAll(t, "commands/init.md", text,
		"nerv-release init", "nerv-release is installed",
		"nerv-release is not",
		"run the CLI command directly for `nerv-task init` and", "`nerv-release init`. A")
	if strings.Contains(text, "until P4") {
		t.Error(`commands/init.md still says release is a nerv extension "until P4"`)
	}
}
