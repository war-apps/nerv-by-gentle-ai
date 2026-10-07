package configure_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
)

func printDeps(home string) configure.Deps {
	return configure.Deps{
		Home:     home,
		FS:       nerv.PluginFS(),
		Runner:   &envtest.FakeRunner{}, // no gentle-ai on PATH in tests
		LookPath: func(string) (string, error) { return "", errNotFound },
	}
}

var errNotFound = errTest("not found")

// ---------------------------------------------------------------------------
// print-has-config-path / print-has-exists-true / print-has-prerequisites-shape
// ---------------------------------------------------------------------------

func TestPrint_ConfigPathAndExists(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)
	paths := configure.Paths{Config: configPath, State: filepath.Join(home, "state.json"), SkillsDir: filepath.Join(home, "skills")}

	result, err := configure.Print(printDeps(home), paths)
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	if result.ConfigPath != configPath {
		t.Errorf("ConfigPath = %q, want %q", result.ConfigPath, configPath)
	}
	if !result.Exists {
		t.Error("Exists = false, want true")
	}
}

func TestPrint_MissingConfig_ExistsFalse(t *testing.T) {
	home := t.TempDir()
	paths := configure.ResolvePaths(home, "")

	result, err := configure.Print(printDeps(home), paths)
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	if result.Exists {
		t.Error("Exists = true, want false")
	}
}

func TestPrint_PrerequisitesShape(t *testing.T) {
	home := t.TempDir()
	paths := configure.ResolvePaths(home, "")

	result, err := configure.Print(printDeps(home), paths)
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	if result.Prerequisites.GentleAI.Found {
		t.Error("GentleAI.Found = true, want false (no fake response)")
	}
	if result.Prerequisites.Engram.Found {
		t.Error("Engram.Found = true, want false (LookPath fails)")
	}
	if result.Prerequisites.Claude.Found {
		t.Error("Claude.Found = true, want false (LookPath fails)")
	}
}

// ---------------------------------------------------------------------------
// print-values-git-base-branch / print-values-stage-inDev /
// print-defaults-git-worktree
// ---------------------------------------------------------------------------

func TestPrint_ValuesAndDefaults(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)
	paths := configure.Paths{Config: configPath, State: filepath.Join(home, "state.json"), SkillsDir: filepath.Join(home, "skills")}

	result, err := configure.Print(printDeps(home), paths)
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	values := map[string]string{}
	for _, e := range result.Values {
		values[e.Key] = e.Value
	}
	if values["git.base_branch"] != "develop" {
		t.Errorf("values['git.base_branch'] = %q, want develop", values["git.base_branch"])
	}
	if values["tasks.providers.teamwork.stages.inDev"] != "DESARROLLO" {
		t.Errorf("values['...stages.inDev'] = %q, want DESARROLLO", values["tasks.providers.teamwork.stages.inDev"])
	}

	defaults := map[string]string{}
	for _, e := range result.Defaults {
		defaults[e.Key] = e.Value
	}
	if defaults["git.worktree"] != "ask" {
		t.Errorf("defaults['git.worktree'] = %q, want ask", defaults["git.worktree"])
	}
	if defaults["git.worktree_pattern"] != ".claude/worktrees/{slug}" {
		t.Errorf("defaults['git.worktree_pattern'] = %q, want .claude/worktrees/{slug}", defaults["git.worktree_pattern"])
	}
}

// ---------------------------------------------------------------------------
// print-models-is-array-with-known-role / print-models-misato-is-override
// ---------------------------------------------------------------------------

func TestPrint_ModelsTableHasOverrideRole(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)
	paths := configure.Paths{Config: configPath, State: filepath.Join(home, "state.json"), SkillsDir: filepath.Join(home, "skills")}

	result, err := configure.Print(printDeps(home), paths)
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	var misato *configure.ModelRow
	for i := range result.Models {
		if result.Models[i].Role == "misato" {
			misato = &result.Models[i]
		}
	}
	if misato == nil {
		t.Fatal("misato not present in models table")
	}
	if misato.Source != "override" {
		t.Errorf("misato.Source = %q, want override", misato.Source)
	}
	if misato.Model != "fable" || misato.Effort != "high" {
		t.Errorf("misato = %+v, want model=fable effort=high", misato)
	}
}

// The role catalogue's purpose and gentle-ai equivalent ride along on every
// models row as additive JSON fields.
func TestPrint_ModelsRowsCarryPurposeAndEquivalent(t *testing.T) {
	home := t.TempDir()
	paths := configure.ResolvePaths(home, "")

	result, err := configure.Print(printDeps(home), paths)
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	byRole := map[string]map[string]any{}
	for _, row := range decoded.Models {
		for _, key := range []string{"purpose", "gentle_ai_equivalent", "gentle_ai_equivalents", "from_phase"} {
			if _, ok := row[key]; !ok {
				t.Errorf("row %v missing key %q", row["role"], key)
			}
		}
		role, ok := row["role"].(string)
		if !ok {
			t.Fatalf("row %v has no string role", row)
		}
		byRole[role] = row
	}
	if _, ok := byRole["kaji-security"]; ok {
		t.Errorf("kaji-security is still a models row; the role was removed")
	}
	if got := byRole["casper"]["gentle_ai_equivalent"]; got != "review-readability" {
		t.Errorf("casper gentle_ai_equivalent = %v, want review-readability", got)
	}
	if got := byRole["kaji-resilience"]["gentle_ai_equivalent"]; got != "review-resilience" {
		t.Errorf("kaji-resilience gentle_ai_equivalent = %v, want review-resilience", got)
	}
	if got := byRole["kaji-resilience"]["from_phase"]; got != "" {
		t.Errorf("kaji-resilience from_phase = %v, want empty (review-resilience is not a phase key)", got)
	}
	if got := byRole["casper"]["from_phase"]; got != "" {
		t.Errorf("casper from_phase = %v, want empty (review-readability is not a phase key)", got)
	}
	if got := byRole["balthasar"]["from_phase"]; got != "jd-judge-a" {
		t.Errorf("balthasar from_phase = %v, want jd-judge-a", got)
	}
	if got := byRole["kaworu"]["gentle_ai_equivalent"]; got != "jd-fix-agent" {
		t.Errorf("kaworu gentle_ai_equivalent = %v, want jd-fix-agent", got)
	}
	// A role covering two gentle-ai agents lists both; the singular field
	// stays a string (the entries joined with ", ") for older readers.
	if got := byRole["melchor"]["gentle_ai_equivalent"]; got != "jd-judge-b, review-risk" {
		t.Errorf("melchor gentle_ai_equivalent = %v, want \"jd-judge-b, review-risk\"", got)
	}
	if got := fmt.Sprint(byRole["melchor"]["gentle_ai_equivalents"]); got != "[jd-judge-b review-risk]" {
		t.Errorf("melchor gentle_ai_equivalents = %v, want [jd-judge-b review-risk]", got)
	}
	if got, ok := byRole["fuyutsuki"]["gentle_ai_equivalents"].([]any); !ok || len(got) != 0 {
		t.Errorf("fuyutsuki gentle_ai_equivalents = %#v, want an empty array", byRole["fuyutsuki"]["gentle_ai_equivalents"])
	}
	if got := byRole["kaworu"]["purpose"]; got != "writes the failing tests first" {
		t.Errorf("kaworu purpose = %v", got)
	}
	if got := byRole["fuyutsuki"]["gentle_ai_equivalent"]; got != "" {
		t.Errorf("fuyutsuki gentle_ai_equivalent = %v, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// print-skills-status-is-array
// ---------------------------------------------------------------------------

func TestPrint_SkillsStatusNonEmpty(t *testing.T) {
	home := t.TempDir()
	paths := configure.ResolvePaths(home, "")

	result, err := configure.Print(printDeps(home), paths)
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}
	if len(result.SkillsStatus) == 0 {
		t.Error("SkillsStatus is empty, want at least one entry")
	}
}

// ---------------------------------------------------------------------------
// JSON shape: field names match the documented -Print contract.
// ---------------------------------------------------------------------------

func TestPrint_JSONFieldNames(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)
	paths := configure.Paths{Config: configPath, State: filepath.Join(home, "state.json"), SkillsDir: filepath.Join(home, "skills")}

	result, err := configure.Print(printDeps(home), paths)
	if err != nil {
		t.Fatalf("Print() error = %v", err)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, key := range []string{"config_path", "exists", "prerequisites", "values", "defaults", "models", "skills_status"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("JSON missing top-level key %q (decoded=%v)", key, decoded)
		}
	}

	prereq, ok := decoded["prerequisites"].(map[string]any)
	if !ok {
		t.Fatal("prerequisites is not an object")
	}
	for _, key := range []string{"gentle_ai", "engram", "claude"} {
		if _, ok := prereq[key]; !ok {
			t.Errorf("prerequisites missing key %q", key)
		}
	}

	models, ok := decoded["models"].([]any)
	if !ok || len(models) == 0 {
		t.Fatal("models is not a non-empty array")
	}
	row, ok := models[0].(map[string]any)
	if !ok {
		t.Fatal("models[0] is not an object")
	}
	for _, key := range []string{"role", "model", "effort", "source"} {
		if _, ok := row[key]; !ok {
			t.Errorf("models[0] missing key %q", key)
		}
	}
}
