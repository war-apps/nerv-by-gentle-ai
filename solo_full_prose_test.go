package nerv

import (
	"io/fs"
	"strings"
	"testing"
)

// Only FULL runs create the ODD feature document, its Engram copy, the
// odd/{feature}/nerv/ run folder and the close commit; DIRECT and LIGHT
// keep their run files in the runtime folder (odd/tasks/ship-deploy-plugins.md,
// L20 "Solo FULL (Recommended)", plus the L18 advisories). Each test pins
// one rule.

const (
	soloDirectRef  = "skills/nerv-orchestrator/references/pipeline-direct.md"
	soloLightRef   = "skills/nerv-orchestrator/references/pipeline-light.md"
	soloFullRef    = "skills/nerv-orchestrator/references/pipeline-full.md"
	soloODDRef     = "skills/nerv-orchestrator/references/odd-document.md"
	soloArtifacts  = "skills/_shared/nerv-artifacts.md"
	soloRunFolder  = ".nerv/runs/{run-id}/"
	soloCloseLabel = "docs: close nerv run for {feature}"
)

// DIRECT and LIGHT never mention creating the ODD document or a close commit.
func TestSoloFull_DirectAndLightCreateNoODDDocumentOrCloseCommit(t *testing.T) {
	for _, path := range []string{soloDirectRef, soloLightRef} {
		text := normalizeSpace(readPluginText(t, path))
		for _, banned := range []string{"odd/tasks/", "odd/{feature}", "close commit", "docs: close nerv run"} {
			if strings.Contains(text, banned) {
				t.Errorf("%s still mentions %q; DIRECT and LIGHT create no ODD document and no close commit", path, banned)
			}
		}
		assertContainsAll(t, path, text, "no ODD feature document", "only its work commit")
	}
}

// FULL does create them, and the ODD reference says only FULL does.
func TestSoloFull_FullCreatesTheODDDocumentAndCloseCommit(t *testing.T) {
	assertPluginContains(t, soloFullRef, "`odd/tasks/{feature}.md`", "`odd/{feature}/nerv/`", soloCloseLabel)
	assertPluginContains(t, soloODDRef, "Only FULL runs create", "`odd/tasks/{feature}.md`", "`odd/{feature}/tasks`", soloCloseLabel)
	assertPluginContains(t, orchestratorCore, "Only FULL runs create")
}

// DIRECT and LIGHT keep their run files in the runtime folder, never committed.
func TestSoloFull_DirectAndLightRunFilesLiveUnderNervRuns(t *testing.T) {
	for _, path := range []string{soloDirectRef, soloLightRef, soloODDRef, soloArtifacts, orchestratorCore} {
		assertPluginContains(t, path, soloRunFolder)
	}
	assertPluginContains(t, soloODDRef, ".nerv/locks/{run-id}.lock")
}

// The task plugin's replay finds the task-events file in either layout.
func TestSoloFull_ReplayFindsTaskEventsInEitherLayout(t *testing.T) {
	for _, path := range []string{"task/commands/replay.md", "task/skills/task/SKILL.md"} {
		data, err := fs.ReadFile(ExtensionsFS(), path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		assertContainsAll(t, path, normalizeSpace(string(data)),
			"`odd/{feature}/nerv/task-events.yaml`", "`.nerv/runs/{run-id}/task-events.yaml`")
	}
	assertPluginContains(t, "skills/nerv-orchestrator/references/task-events.md",
		"`odd/{feature}/nerv/task-events.yaml`", "`.nerv/runs/{run-id}/task-events.yaml`")
}

// Only Aoba writes the Commit column: every sentence naming the column
// together with Ikari also names Aoba as its writer.
func TestSoloFull_OnlyAobaWritesTheCommitColumn(t *testing.T) {
	const rule = "Aoba writes the `Commit` column"
	for _, path := range []string{"agents/aoba.md", soloFullRef, soloODDRef} {
		assertPluginContains(t, path, rule)
	}
	err := fs.WalkDir(PluginFS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		for _, sentence := range strings.Split(normalizeSpace(readPluginText(t, path)), ". ") {
			if strings.Contains(sentence, "`Commit`") && strings.Contains(sentence, "Ikari") && !strings.Contains(sentence, "Aoba") {
				t.Errorf("%s gives Ikari the `Commit` column: %q", path, sentence)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the plugin tree: %v", err)
	}
}

// MAGI members and auditors judge each round independently, and the vote
// reading lists agree between the pipeline and the agent files.
func TestSoloFull_MAGIReadsOnlyItsOwnListBlind(t *testing.T) {
	const blind = "never the `MAGI` column, `nerv/votes.md`, an earlier round's audit report or pass files, or the gate and ruling `## Log` entries"
	for _, path := range []string{soloFullRef, "agents/balthasar.md", "agents/melchior.md", "agents/casper.md", "agents/gendo.md"} {
		assertPluginContains(t, path, blind)
	}
	lists := map[string]string{
		"agents/balthasar.md": "the design `## Log` entry, `## Tasks` and `nerv/criticality.md`",
		"agents/melchior.md":  "the design `## Log` entry, `## Tasks`, `## Specs` and `nerv/criticality.md`",
		"agents/casper.md":    "`## Specs`, the test-plan `## Log` entry, `## Tasks` and `nerv/criticality.md`",
	}
	for path, list := range lists {
		assertPluginContains(t, path, list)
		assertPluginContains(t, soloFullRef, list)
	}
}
