package nerv

import (
	"io/fs"
	"testing"
)

// /nerv:init ends by offering every setup flow that exists on the machine:
// NERV's own configuration, the tasks section, the release section and the
// deploy section, each run by following its command file.
func TestInitCommand_OffersSetupFlows(t *testing.T) {
	data, err := fs.ReadFile(PluginFS(), "commands/init.md")
	if err != nil {
		t.Fatalf("reading plugin/commands/init.md: %v", err)
	}
	assertContainsAll(t, "commands/init.md", string(data),
		"/nerv:configure", "/task:setup", "/release:setup", "/release:deploy-setup",
		"multi-select", "Follow the", "not offered")
}
