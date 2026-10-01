package nerv

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// referencesTokenPattern matches a references/<name>.md token as it would
// appear in SKILL.md prose, e.g. "references/pipeline-light.md".
var referencesTokenPattern = regexp.MustCompile(`references/[A-Za-z0-9_-]+\.md`)

// TestOrchestratorSkillCoreStaysWithinSkillGuidance guards the split of
// plugin/skills/nerv-orchestrator/SKILL.md into a core under 500 lines plus
// reference files loaded on demand (see odd/tasks/protocol-injection.md,
// task T2). It checks: the core's line count, that every references/*.md
// token the core mentions actually exists, that every file physically
// present under references/ is mentioned by the core (no orphans), and that
// the references/ directory exists and is non-empty.
func TestOrchestratorSkillCoreStaysWithinSkillGuidance(t *testing.T) {
	pfs := PluginFS()

	const corePath = "skills/nerv-orchestrator/SKILL.md"
	coreBytes, err := fs.ReadFile(pfs, corePath)
	if err != nil {
		t.Fatalf("reading %s: %v", corePath, err)
	}
	core := string(coreBytes)

	lineCount := strings.Count(core, "\n")
	if !strings.HasSuffix(core, "\n") && len(core) > 0 {
		lineCount++
	}
	const maxLines = 500
	if lineCount > maxLines {
		t.Errorf("SKILL.md has %d lines, want at most %d", lineCount, maxLines)
	}

	const referencesDir = "skills/nerv-orchestrator/references"
	entries, err := fs.ReadDir(pfs, referencesDir)
	if err != nil {
		t.Fatalf("reading dir %s: %v", referencesDir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected %s to be non-empty", referencesDir)
	}

	present := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		present[e.Name()] = true
	}

	mentioned := make(map[string]bool)
	for _, tok := range referencesTokenPattern.FindAllString(core, -1) {
		name := strings.TrimPrefix(tok, "references/")
		mentioned[name] = true

		if !present[name] {
			t.Errorf("SKILL.md mentions %s, but no such file exists under %s", tok, referencesDir)
		}
	}

	for name := range present {
		if !mentioned[name] {
			t.Errorf("file %s/%s exists but is never mentioned by SKILL.md as references/%s", referencesDir, name, name)
		}
	}
}
