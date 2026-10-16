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

func readPluginText(t *testing.T, path string) string {
	t.Helper()
	data, err := fs.ReadFile(PluginFS(), path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func assertContainsAll(t *testing.T, path, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("%s does not contain %q", path, want)
		}
	}
}

// The orchestrator routes every request through three tiers that mirror
// gentle-ai's Work Routing Ladder: DIRECT (direct inline), LIGHT (delegated
// direct) and FULL (NERV's own governed pipeline), with a one-way ratchet
// and a route declaration logged per run (odd/tasks/orchestrator-routing-tiers.md).
func TestOrchestrator_ClassifiesThreeRoutingTiers(t *testing.T) {
	const path = "skills/nerv-orchestrator/SKILL.md"
	core := readPluginText(t, path)
	assertContainsAll(t, path, core,
		"## Classification: DIRECT, LIGHT or FULL",
		"references/pipeline-direct.md",
		"DIRECT → LIGHT → FULL",
		"route declaration",
		"one parallel batch",
		"approximately 10k tokens",
		"more than approximately 5 sequential lookups",
	)
	if strings.Contains(core, "4-file mapping") || strings.Contains(core, "20-call backstop") {
		t.Errorf("%s still carries the retired 4-file/20-call delegation triggers", path)
	}
}

// LIGHT launches one bounded writer that runs its own test-first cycle;
// Kaworu's separate RED step belongs to FULL only, and Maya is launched only
// when the Delegated Verification Gate asks for an independent verifier.
func TestLightPipeline_LaunchesOneBoundedWriter(t *testing.T) {
	const path = "skills/nerv-orchestrator/references/pipeline-light.md"
	light := readPluginText(t, path)
	assertContainsAll(t, path, light, "MODE: solo", "Delegated Verification Gate", "spot check", "user validates before commit")
	for _, line := range strings.Split(light, "\n") {
		if strings.HasPrefix(line, "|") && strings.Contains(line, "Kaworu") {
			t.Errorf("%s still has a Kaworu step row: %s", path, line)
		}
	}
}

// DIRECT is the only tier where Ikari edits source itself, and it still
// relays the commit validation to the user before committing.
func TestDirectPipeline_IkariEditsInlineAndUserValidatesCommit(t *testing.T) {
	const path = "skills/nerv-orchestrator/references/pipeline-direct.md"
	direct := readPluginText(t, path)
	assertContainsAll(t, path, direct, "Ikari", "user validates before commit", "RDD hook", "ratchet")
}

// Every domain pilot documents the LIGHT solo mode in which it writes its
// own failing test before GREEN.
func TestPilots_DocumentSoloMode(t *testing.T) {
	for _, pilot := range []string{"shinji", "asuka", "rei", "toji"} {
		path := "agents/" + pilot + ".md"
		assertContainsAll(t, path, readPluginText(t, path), "### MODE: solo (LIGHT path)")
	}
}

// FULL's fix routing names its own work-unit cycle instead of borrowing the
// LIGHT one, so changing LIGHT never changes FULL.
func TestFullPipeline_FixRoutingSpellsOutItsOwnCycle(t *testing.T) {
	const path = "skills/nerv-orchestrator/references/pipeline-full.md"
	if strings.Contains(readPluginText(t, path), "LIGHT work-unit cycle") {
		t.Errorf("%s still routes fixes through the LIGHT work-unit cycle", path)
	}
}

// The shared contracts know the DIRECT tier: state.yaml records it and the
// tracker's testing move follows the work unit's verification.
func TestSharedContracts_KnowTheDirectTier(t *testing.T) {
	const artifacts = "skills/_shared/nerv-artifacts.md"
	assertContainsAll(t, artifacts, readPluginText(t, artifacts), "# direct | light | full")
	const tasks = "skills/nerv-tasks/SKILL.md"
	assertContainsAll(t, tasks, readPluginText(t, tasks), "when the work unit's verification starts")
}
