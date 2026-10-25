package nerv

import "testing"

// Learnings from real NERV runs, folded into the core prose
// (odd/tasks/tasks-plugin-split.md, T6, S19-S23). Each test pins the key
// phrases of one rule in the file that owns it.

const (
	orchestratorCore   = "skills/nerv-orchestrator/SKILL.md"
	launchDiscipline   = "skills/nerv-orchestrator/references/launch-discipline.md"
	usageAndLog        = "skills/nerv-orchestrator/references/usage-and-log.md"
	deliveryRef        = "skills/nerv-orchestrator/references/delivery.md"
	rddRelayRef        = "skills/nerv-orchestrator/references/rdd-relay.md"
	pipelineLightRef   = "skills/nerv-orchestrator/references/pipeline-light.md"
	pipelineFullRef    = "skills/nerv-orchestrator/references/pipeline-full.md"
	phaseCommon        = "skills/_shared/nerv-phase-common.md"
	artifactsContract  = "skills/_shared/nerv-artifacts.md"
	runSummaryColumns  = "| agent | model | tokens_in | tokens_out | tokens_total | duration_s |"
	pendingMirrorLabel = "Pending KB mirror"
)

func assertPluginContains(t *testing.T, path string, wants ...string) {
	t.Helper()
	assertContainsAll(t, path, normalizeSpace(readPluginText(t, path)), wants...)
}

// S19: rejected sub-agent mem_save calls are partial, handed back, and
// flushed by the orchestrator at close; leftovers are listed in the summary.
func TestS19_OrchestratorFlushesRejectedKBMirrorsAtClose(t *testing.T) {
	assertPluginContains(t, phaseCommon,
		"session registration could not be confirmed",
		"`partial`, not `blocked`",
		pendingMirrorLabel)
	for _, agent := range []string{"misato", "kaji", "fuyutsuki"} {
		assertPluginContains(t, "agents/"+agent+".md", pendingMirrorLabel, "`partial`, not `blocked`")
	}
	assertPluginContains(t, usageAndLog,
		"Engram flush at close",
		"Ikari saves it itself",
		"## KB mirrors unflushed")
	assertPluginContains(t, artifactsContract, "## KB mirrors unflushed")
	assertPluginContains(t, pipelineLightRef, "Engram flush at close")
	assertPluginContains(t, pipelineFullRef, "Engram flush at close")
}

// S20: launch discipline.
func TestS20_LaunchDiscipline(t *testing.T) {
	assertPluginContains(t, orchestratorCore, "references/launch-discipline.md")
	assertPluginContains(t, launchDiscipline,
		"never launches two agents at once in one worktree when one of them is a gate",
		"Ikari never freezes a round itself",
		"appends it verbatim",
		"appended_by: ikari",
		"absolute and scoped to the feature",
		"`nerv/audit/commits-round-N.txt`",
		"never adds wording the user did not approve")
	assertPluginContains(t, pipelineFullRef, "`nerv/audit/commits-round-N.txt`")
	assertPluginContains(t, "agents/aoba.md", "commits-round-N.txt")
}

// S21: close robustness — a LIGHT close in its runtime folder with no
// archive and no close commit, a FULL close commit on the ODD document,
// and a lock heartbeat (the RENAMED-delta rule went with OpenSpec in
// odd/tasks/ship-deploy-plugins.md, S10; the LIGHT close commit went with
// L20, "Solo FULL").
func TestS21_CloseRobustness(t *testing.T) {
	assertPluginContains(t, pipelineLightRef, "| 9. Close |", "There is no archive", "A LIGHT run is never left delivered but unclosed", "`closed_at`", ".nerv/locks/{run-id}.lock")
	assertPluginContains(t, pipelineFullRef, "docs: close nerv run for {feature}")
	assertPluginContains(t, orchestratorCore,
		"at every phase transition",
		"report it as stale, not active")
	assertPluginContains(t, "commands/status.md", "stale, not active")
}

// S22: repos that git-ignore the artifacts, the lock never staged, and the
// RDD assess integration excluding NERV runtime folders on its own.
func TestS22_ArtifactsThatGitIgnores(t *testing.T) {
	assertPluginContains(t, deliveryRef,
		"git check-ignore",
		"the artifacts stay local",
		"no archive commit")
	assertPluginContains(t, "agents/aoba.md",
		"The orchestrator lock `.nerv/locks/{run-id}.lock` is never staged",
		"git check-ignore")
	assertPluginContains(t, rddRelayRef,
		"--untracked-scope=select",
		"--intended-untracked=",
		"--untracked-scope=exclude",
		"never asks the user to edit `.git/info/exclude`",
		"no per-path exclude flag")
}

// S23: per-launch telemetry with model, tokens in and out, and duration.
func TestS23_TelemetryRecordsModelTokensAndDuration(t *testing.T) {
	assertPluginContains(t, usageAndLog,
		"tokens_in", "tokens_out", "duration_ms", "total_tokens",
		"subagents/agent-", "frontmatter `model:`")
	assertPluginContains(t, artifactsContract, runSummaryColumns)
	assertPluginContains(t, "agents/aoba.md", "`agent, model, tokens_in, tokens_out, tokens_total, duration_s`")
}
