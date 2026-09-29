package skills_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	nerv "github.com/war-apps/nerv-gentle-ai"
	"github.com/war-apps/nerv-gentle-ai/internal/env/envtest"
	"github.com/war-apps/nerv-gentle-ai/internal/skills"
)

// ---------------------------------------------------------------------------
// LoadManifest / LoadManifestFS — mirrors Read-NervSkillsManifest.
// ---------------------------------------------------------------------------

func TestLoadManifestFS_RealEmbeddedManifest(t *testing.T) {
	manifest, err := skills.LoadManifestFS(nerv.PluginFS())
	if err != nil {
		t.Fatalf("LoadManifestFS() error = %v", err)
	}
	if manifest.Schema != "nerv.skills-manifest/v1" {
		t.Fatalf("Schema = %q, want nerv.skills-manifest/v1", manifest.Schema)
	}
	if len(manifest.Skills) < 15 {
		t.Fatalf("len(Skills) = %d, want >= 15", len(manifest.Skills))
	}

	externalCount := 0
	names := map[string]bool{}
	for _, s := range manifest.Skills {
		names[s.Name] = true
		if s.Kind == skills.KindExternal {
			externalCount++
		}
	}
	if externalCount != 9 {
		t.Fatalf("external count = %d, want 9", externalCount)
	}
	for _, want := range []string{
		"tdd", "playwright-best-practices", "dotnet-best-practices",
		"typescript-best-practices", "best-practices", "solid-principles",
		"clean-code-guard", "hexagonal-architecture", "c4-architecture",
	} {
		if !names[want] {
			t.Fatalf("manifest missing external skill %q", want)
		}
	}
}

func TestLoadManifest_WrongSchemaErrors(t *testing.T) {
	_, err := skills.LoadManifest([]byte(`{ "schema": "wrong-schema/v1", "skills": [] }`))
	if err == nil {
		t.Fatal("LoadManifest() error = nil, want error for wrong schema")
	}
}

func TestLoadManifest_EntryMissingRequiredKeyErrors(t *testing.T) {
	_, err := skills.LoadManifest([]byte(`{ "schema": "nerv.skills-manifest/v1", "skills": [ { "name": "x" } ] }`))
	if err == nil {
		t.Fatal("LoadManifest() error = nil, want error for a missing 'kind' key")
	}
}

func TestLoadManifest_ExternalWithoutRepoErrors(t *testing.T) {
	_, err := skills.LoadManifest([]byte(`{ "schema": "nerv.skills-manifest/v1", "skills": [ { "name": "x", "kind": "external", "used_by": ["a"] } ] }`))
	if err == nil {
		t.Fatal("LoadManifest() error = nil, want error for external kind without repo")
	}
}

func TestLoadManifest_UnknownKindErrors(t *testing.T) {
	_, err := skills.LoadManifest([]byte(`{ "schema": "nerv.skills-manifest/v1", "skills": [ { "name": "x", "kind": "bogus", "used_by": ["a"] } ] }`))
	if err == nil {
		t.Fatal("LoadManifest() error = nil, want error for unknown kind")
	}
}

// ---------------------------------------------------------------------------
// IsInstalled — mirrors Test-NervSkillInstalled.
// ---------------------------------------------------------------------------

func TestIsInstalled_TrueWhenSkillMdPresent(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "present-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# present"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !skills.IsInstalled(dir, "present-skill") {
		t.Fatal("IsInstalled() = false, want true")
	}
}

func TestIsInstalled_FalseWhenSkillMdAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "empty-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	if skills.IsInstalled(dir, "empty-skill") {
		t.Fatal("IsInstalled() = true, want false")
	}
}

func TestIsInstalled_FalseWhenDirAbsent(t *testing.T) {
	dir := t.TempDir()

	if skills.IsInstalled(dir, "nonexistent-skill") {
		t.Fatal("IsInstalled() = true, want false")
	}
}

// ---------------------------------------------------------------------------
// Status — mirrors Get-NervSkillsStatus.
// ---------------------------------------------------------------------------

func TestStatus_ComputesActionPerKind(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"tdd", "solid-principles"} {
		skillDir := filepath.Join(dir, n)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# "+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	manifest := &skills.Manifest{Schema: "nerv.skills-manifest/v1", Skills: []skills.SkillEntry{
		{Name: "tdd", Kind: skills.KindExternal, Repo: "mattpocock/skills", Skill: "tdd", UsedBy: []string{"testing"}},
		{Name: "solid-principles", Kind: skills.KindExternal, Repo: "thebushidocollective/han", Skill: "solid-principles", UsedBy: []string{"best-practices"}},
		{Name: "c4-architecture", Kind: skills.KindExternal, Repo: "softaworks/agent-toolkit", UsedBy: []string{"architecture"}},
		{Name: "work-unit-commits", Kind: skills.KindGentleAI, UsedBy: []string{"delivery"}},
		{Name: "security-review", Kind: skills.KindBuiltin, UsedBy: []string{"audit"}},
	}}

	statuses := skills.Status(manifest, dir)
	byName := map[string]skills.SkillStatus{}
	for _, s := range statuses {
		byName[s.Name] = s
	}

	if s := byName["tdd"]; !s.Installed || s.Action != skills.ActionNone {
		t.Fatalf("tdd = %+v, want Installed=true Action=none", s)
	}
	if s := byName["solid-principles"]; !s.Installed || s.Action != skills.ActionNone {
		t.Fatalf("solid-principles = %+v, want Installed=true Action=none", s)
	}
	if s := byName["c4-architecture"]; s.Installed || s.Action != skills.ActionInstall {
		t.Fatalf("c4-architecture = %+v, want Installed=false Action=install", s)
	}
	if s := byName["work-unit-commits"]; s.Installed || s.Action != skills.ActionVerifyGentleAI {
		t.Fatalf("work-unit-commits = %+v, want Installed=false Action=verify-gentle-ai", s)
	}
	if s := byName["security-review"]; s.Action != skills.ActionNone {
		t.Fatalf("security-review.Action = %q, want none", s.Action)
	}
}

// ---------------------------------------------------------------------------
// InstallArgs — mirrors New-NervSkillInstallArgs.
// ---------------------------------------------------------------------------

func TestInstallArgs_WithSkill(t *testing.T) {
	got := skills.InstallArgs("mattpocock/skills", "tdd")
	want := []string{"skills", "add", "mattpocock/skills", "--skill", "tdd", "-g", "-a", "claude-code", "-y"}
	assertStringSlicesEqual(t, got, want)
}

func TestInstallArgs_WithoutSkill(t *testing.T) {
	got := skills.InstallArgs("currents-dev/playwright-best-practices-skill", "")
	want := []string{"skills", "add", "currents-dev/playwright-best-practices-skill", "-g", "-a", "claude-code", "-y"}
	assertStringSlicesEqual(t, got, want)
}

func assertStringSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// InstallPlan
// ---------------------------------------------------------------------------

func TestInstallPlan_BuildsInstallsAndRemedies(t *testing.T) {
	statuses := []skills.SkillStatus{
		{Name: "tdd", Kind: skills.KindExternal, Installed: true, Action: skills.ActionNone},
		{Name: "c4-architecture", Kind: skills.KindExternal, Repo: "softaworks/agent-toolkit", Skill: "c4-architecture", Installed: false, Action: skills.ActionInstall},
		{Name: "work-unit-commits", Kind: skills.KindGentleAI, Installed: false, Action: skills.ActionVerifyGentleAI},
	}

	plan := skills.InstallPlan(statuses)

	if len(plan.Installs) != 1 || plan.Installs[0].Name != "c4-architecture" {
		t.Fatalf("Installs = %+v, want exactly c4-architecture", plan.Installs)
	}
	wantArgs := []string{"skills", "add", "softaworks/agent-toolkit", "--skill", "c4-architecture", "-g", "-a", "claude-code", "-y"}
	assertStringSlicesEqual(t, plan.Installs[0].Args, wantArgs)

	if len(plan.Remedies) != 1 {
		t.Fatalf("Remedies = %v, want exactly one", plan.Remedies)
	}
	if want := "Remedy: run 'gentle-ai install' (or 'gentle-ai sync') to provide gentle-ai skill 'work-unit-commits'."; plan.Remedies[0] != want {
		t.Fatalf("Remedies[0] = %q, want %q", plan.Remedies[0], want)
	}
}

// ---------------------------------------------------------------------------
// FilterOnly — the -Only unknown-name validation.
// ---------------------------------------------------------------------------

func TestFilterOnly_UnknownNameErrorsWithName(t *testing.T) {
	manifest := &skills.Manifest{Skills: []skills.SkillEntry{{Name: "tdd", Kind: skills.KindExternal, Repo: "x"}}}

	_, err := skills.FilterOnly(manifest, []string{"no-such-skill"})
	if err == nil {
		t.Fatal("FilterOnly() error = nil, want error for an unknown name")
	}
	if got := err.Error(); !contains(got, "no-such-skill") {
		t.Fatalf("error = %q, want it to name the unknown skill", got)
	}
}

func TestFilterOnly_KnownNamesRestrictEntries(t *testing.T) {
	manifest := &skills.Manifest{Skills: []skills.SkillEntry{
		{Name: "tdd", Kind: skills.KindExternal, Repo: "a"},
		{Name: "solid-principles", Kind: skills.KindExternal, Repo: "b"},
	}}

	got, err := skills.FilterOnly(manifest, []string{"tdd"})
	if err != nil {
		t.Fatalf("FilterOnly() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "tdd" {
		t.Fatalf("FilterOnly() = %+v, want exactly tdd", got)
	}
}

func TestFilterOnly_EmptyReturnsAllEntries(t *testing.T) {
	manifest := &skills.Manifest{Skills: []skills.SkillEntry{
		{Name: "tdd", Kind: skills.KindExternal, Repo: "a"},
		{Name: "solid-principles", Kind: skills.KindExternal, Repo: "b"},
	}}

	got, err := skills.FilterOnly(manifest, nil)
	if err != nil {
		t.Fatalf("FilterOnly() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("FilterOnly() = %+v, want both entries", got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Install — executes external installs through env.Runner (FakeRunner).
// ---------------------------------------------------------------------------

func TestInstall_RunsNpxForEachInstallStep(t *testing.T) {
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"npx": {ExitCode: 0},
	}}
	plan := skills.Plan{Installs: []skills.InstallStep{
		{Name: "tdd", Args: []string{"skills", "add", "mattpocock/skills", "--skill", "tdd", "-g", "-a", "claude-code", "-y"}},
	}}

	result := skills.Install(context.Background(), runner, plan)

	if result.Installed != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v, want Installed=1 Failed=0", result)
	}
	if len(runner.Calls) != 1 || runner.Calls[0].Name != "npx" {
		t.Fatalf("Calls = %+v, want exactly one npx call", runner.Calls)
	}
	if len(runner.Calls[0].Args) != 9 || runner.Calls[0].Args[0] != "skills" {
		t.Fatalf("Calls[0].Args = %v, unexpected", runner.Calls[0].Args)
	}
}

func TestInstall_CountsNonZeroExitAsFailure(t *testing.T) {
	runner := &envtest.FakeRunner{Responses: map[string]envtest.Response{
		"npx": {ExitCode: 1},
	}}
	plan := skills.Plan{Installs: []skills.InstallStep{
		{Name: "broken-skill", Args: []string{"skills", "add", "nowhere/nothing", "-g", "-a", "claude-code", "-y"}},
	}}

	result := skills.Install(context.Background(), runner, plan)

	if result.Installed != 0 || result.Failed != 1 || len(result.Failures) != 1 || result.Failures[0] != "broken-skill" {
		t.Fatalf("result = %+v, want one recorded failure for broken-skill", result)
	}
}
