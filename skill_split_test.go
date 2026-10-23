package nerv

import (
	"io/fs"
	"os"
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

// DIRECT is the only tier where Ikari edits source itself. The test pins its
// real rules: the classification thresholds in the core, no NERV agent
// besides Maya (the task plugin's dispatcher is not a NERV agent), Maya only on a high or unassessable tier with RDD off or
// unknown, user validation before the commit, and the ratchet to LIGHT.
func TestDirectPipeline_PinsItsRoutingRules(t *testing.T) {
	const corePath = "skills/nerv-orchestrator/SKILL.md"
	assertContainsAll(t, corePath, normalizeSpace(readPluginText(t, corePath)),
		"The change is **DIRECT** (direct inline) when no FULL criterion holds",
		"at most 3 calls and approximately 10k tokens",
		"one mechanical, already-understood file",
		"needing no research and no unresolved design decision",
	)

	const path = "skills/nerv-orchestrator/references/pipeline-direct.md"
	direct := readPluginText(t, path)
	assertContainsAll(t, path, normalizeSpace(direct),
		"No NERV agent is launched except `nerv:maya`",
		"`nerv:maya` only when",
		"RDD off or unknown",
		"`high`, a failed assessment or an unrecognized verb launches `nerv:maya`",
		"**user validates before commit**",
		"**Ratchet to LIGHT.**",
		"loads `pipeline-light.md`",
		"RDD hook",
	)
	for _, role := range []string{"hyuga", "shinji", "asuka", "rei", "toji", "kaworu", "ritsuko", "aoba", "misato",
		"balthasar", "melchior", "casper", "fuyutsuki", "kaji", "gendo"} {
		if strings.Contains(direct, "nerv:"+role) {
			t.Errorf("%s launches nerv:%s; DIRECT launches only nerv:maya", path, role)
		}
	}
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

// The shared contracts know the DIRECT tier: state.yaml records it. The
// testing move of the task plugin is pinned in task_split_prose_test.go.
func TestSharedContracts_KnowTheDirectTier(t *testing.T) {
	const artifacts = "skills/_shared/nerv-artifacts.md"
	assertContainsAll(t, artifacts, readPluginText(t, artifacts), "# direct | light | full")
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var pilotDisplayNames = map[string]string{"shinji": "Shinji", "asuka": "Asuka", "rei": "Rei", "toji": "Toji"}

// soloBlock returns a pilot's MODE: solo section with its own name and
// pronoun replaced, so the four copies can be compared for drift.
func soloBlock(t *testing.T, pilot string) string {
	t.Helper()
	path := "agents/" + pilot + ".md"
	text := readPluginText(t, path)
	const heading = "### MODE: solo (LIGHT path)"
	start := strings.Index(text, heading)
	if start < 0 {
		t.Fatalf("%s has no %q section", path, heading)
	}
	rest := text[start+len(heading):]
	if end := strings.Index(rest, "\n### "); end >= 0 {
		rest = rest[:end]
	}
	block := strings.ReplaceAll(rest, pilotDisplayNames[pilot], "{pilot}")
	block = regexp.MustCompile(`\b(his|her)\b`).ReplaceAllString(block, "{their}")
	return normalizeSpace(block)
}

// The MODE: solo contract is copied into each pilot (agents read only their
// own file), so the four copies must stay identical modulo name and pronoun.
func TestPilots_SoloModeBlocksAreIdentical(t *testing.T) {
	want := soloBlock(t, "shinji")
	for _, pilot := range []string{"asuka", "rei", "toji"} {
		if got := soloBlock(t, pilot); got != want {
			t.Errorf("agents/%s.md MODE: solo drifted from agents/shinji.md:\n got: %s\nwant: %s", pilot, got, want)
		}
	}
}

// A pilot that says it never commits names Ikari as the committer in solo
// mode, with the same wording in every pilot.
func TestPilots_NeverCommitsNamesIkariInSoloMode(t *testing.T) {
	for _, pilot := range []string{"asuka", "rei", "toji"} {
		path := "agents/" + pilot + ".md"
		assertContainsAll(t, path, normalizeSpace(readPluginText(t, path)),
			"return `next_recommended: aoba-commit` (`ikari-commit` in `MODE: solo`) so Aoba (Ikari in `MODE: solo`) can show the diff")
	}
}

// Maya never hands a LIGHT or DIRECT commit to Aoba: Ikari commits there, and
// her DIRECT-path launch has its own envelope.
func TestMaya_ReducedGateHandsTheCommitToIkari(t *testing.T) {
	const path = "agents/maya.md"
	maya := readPluginText(t, path)
	if strings.Contains(maya, "aoba-commit") {
		t.Errorf("%s still returns aoba-commit; Ikari commits in DIRECT and LIGHT", path)
	}
	assertContainsAll(t, path, normalizeSpace(maya),
		"### MODE: reduced (DIRECT and LIGHT paths)",
		"`next_recommended`: `ikari-commit`",
		"In the DIRECT path",
	)
}

func TestKaworu_InputsHaveNoStaleAlternative(t *testing.T) {
	const path = "agents/kaworu.md"
	if strings.Contains(readPluginText(t, path), "When neither resolves") {
		t.Errorf("%s still says \"When neither resolves\" after LIGHT stopped launching Kaworu", path)
	}
}

// Resume rebuilds DIRECT and LIGHT runs, whose work unit stays uncommitted
// in the tree until Ikari commits it, without ever discarding the tree.
func TestResume_RebuildsDirectAndLightState(t *testing.T) {
	const path = "skills/nerv-orchestrator/references/resume.md"
	assertContainsAll(t, path, normalizeSpace(readPluginText(t, path)),
		"`path: direct`",
		"`path: light`",
		"uncommitted",
		"`commit_recorded`",
		"Never discard",
		"blocking prompt",
	)
}

func TestIntegrationDoc_VerificationGateNamesUnknown(t *testing.T) {
	data, err := os.ReadFile("docs/integration.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := normalizeSpace(string(data))
	assertContainsAll(t, "docs/integration.md", doc,
		"Maya only when RDD is off or unknown and the native assessment is high or unassessable")
	if strings.Contains(doc, "Maya only when RDD is off and") {
		t.Errorf("docs/integration.md says \"RDD is off\" without \"or unknown\"")
	}
}

var graderPatternRe = regexp.MustCompile(`(?m)^pattern: "(.*)"$`)

// The classification graders require an explicit "Classification: <TIER>"
// line, so an option list or "not DIRECT" never passes, and every case asks
// for that exact line.
func TestClassificationGraders_RequireAnExplicitLine(t *testing.T) {
	for _, tier := range []string{"DIRECT", "LIGHT", "FULL"} {
		lower := strings.ToLower(tier)
		dir := "evals/classification-" + lower
		grader := readPluginText(t, dir+"/graders/classifies-"+lower+".md")
		m := graderPatternRe.FindStringSubmatch(grader)
		if m == nil {
			t.Errorf("%s grader has no pattern line", dir)
			continue
		}
		re, err := regexp.Compile(strings.ReplaceAll(m[1], `\\`, `\`))
		if err != nil {
			t.Errorf("%s grader pattern does not compile: %v", dir, err)
			continue
		}
		for _, ok := range []string{"Classification: " + tier, "**Classification:** " + tier + "\nPath: x", "Some text.\nClassification: `" + tier + "`"} {
			if !re.MatchString(ok) {
				t.Errorf("%s grader does not match %q", dir, ok)
			}
		}
		for _, bad := range []string{"print the classification (DIRECT, LIGHT or FULL)", "Classification: not " + tier, "this is " + tier, "Classification: " + tier + "ISH"} {
			if re.MatchString(bad) {
				t.Errorf("%s grader wrongly matches %q", dir, bad)
			}
		}
		assertContainsAll(t, dir+"/case.yaml", readPluginText(t, dir+"/case.yaml"), "`Classification: <TIER>`")
	}
}
