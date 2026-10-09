package nerv_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// launchLabel is the Agent tool description shape every NERV launch uses, so
// Claude Code shows nerv:<role>({Display name}: {action}).
const launchLabel = "`{Display name}: {action}`"

// launchExampleRe matches a backticked "<name>: <action>" example label.
var launchExampleRe = regexp.MustCompile("`([^`{}:]+): [^`{}]+`")

// The orchestrator states the launch label rule, and at least one of its
// examples uses a display name from the role catalogue.
func TestOrchestrator_LaunchDescriptionUsesDisplayName(t *testing.T) {
	const path = "skills/nerv-orchestrator/SKILL.md"
	skill := readPluginFile(t, path)
	if !strings.Contains(skill, launchLabel) {
		t.Fatalf("%s does not state the %s launch description rule", path, launchLabel)
	}
	names := map[string]bool{}
	for _, info := range config.Roles().Info {
		names[info.DisplayName] = true
	}
	for _, m := range launchExampleRe.FindAllStringSubmatch(skill, -1) {
		if names[m[1]] {
			return
		}
	}
	t.Errorf("%s: no `<display name>: <action>` example uses a display name from config.Roles()", path)
}

// Each launch entry of the deliberation log carries the same label.
func TestDeliberationLog_LaunchEntryCarriesLaunchLabel(t *testing.T) {
	for _, path := range []string{
		"skills/nerv-orchestrator/references/usage-and-log.md",
		"skills/_shared/nerv-artifacts.md",
	} {
		if !strings.Contains(readPluginFile(t, path), launchLabel) {
			t.Errorf("%s does not put the %s label on launch entries", path, launchLabel)
		}
	}
	artifacts := readPluginFile(t, "skills/_shared/nerv-artifacts.md")
	if !strings.Contains(artifacts, "| launch | {Display name}: {action}") {
		t.Errorf("nerv-artifacts.md: the launch line template does not start its payload with the launch label")
	}
}
