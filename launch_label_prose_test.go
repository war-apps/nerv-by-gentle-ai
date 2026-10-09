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

// launchLabelBullet returns the SKILL.md bullet that states the launch label
// rule, from its first line to the line before the next bullet or heading.
func launchLabelBullet(t *testing.T, skill string) string {
	t.Helper()
	start := strings.Index(skill, "- Every launch's Agent tool `description` reads "+launchLabel)
	if start < 0 {
		t.Fatalf("SKILL.md does not state the %s launch description rule", launchLabel)
	}
	var b strings.Builder
	for i, line := range strings.Split(skill[start:], "\n") {
		if i > 0 && (strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "#") || line == "") {
			break
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// The orchestrator states the launch label rule, and every example in that
// rule uses a display name from the role catalogue, so the prose cannot drift
// from config.Roles().
func TestOrchestrator_LaunchDescriptionUsesDisplayName(t *testing.T) {
	const path = "skills/nerv-orchestrator/SKILL.md"
	bullet := launchLabelBullet(t, readPluginFile(t, path))
	names := map[string]bool{}
	for _, info := range config.Roles().Info {
		names[info.DisplayName] = true
	}
	examples := launchExampleRe.FindAllStringSubmatch(bullet, -1)
	if len(examples) == 0 {
		t.Fatalf("%s: the launch label rule has no `<display name>: <action>` example", path)
	}
	for _, m := range examples {
		if !names[m[1]] {
			t.Errorf("%s: launch label example %q does not use a display name from config.Roles()", path, m[0])
		}
	}
}

// The action never carries the log line's delimiters, so a launch entry in
// the deliberation log stays unambiguous.
func TestOrchestrator_LaunchActionForbidsLogDelimiters(t *testing.T) {
	bullet := launchLabelBullet(t, readPluginFile(t, "skills/nerv-orchestrator/SKILL.md"))
	if !strings.Contains(bullet, "never contains `|` or `;`") {
		t.Errorf("the launch label rule does not forbid `|` and `;` in the action:\n%s", bullet)
	}
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
