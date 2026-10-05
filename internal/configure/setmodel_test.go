package configure_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
)

// ---------------------------------------------------------------------------
// setmodel round trip: set a role's model/effort, keeping the existing
// role's override, then clear it back to default.
// ---------------------------------------------------------------------------

func TestSetModel_AddsOverrideKeepsExisting(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	result, err := configure.SetModel(deps, paths, []string{"hyuga=opus/xhigh"})
	if err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	want := configure.Change{Key: "models.hyuga", From: "default", To: "opus/xhigh"}
	if len(result.Changes) != 1 || result.Changes[0] != want {
		t.Errorf("Changes = %+v, want [%+v]", result.Changes, want)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "hyuga: { model: opus, effort: xhigh }") {
		t.Error("new role override not present")
	}
	if !strings.Contains(string(after), "misato: { model: fable, effort: high }") {
		t.Error("existing role override not preserved")
	}
}

func TestSetModel_ClearRemovesRole(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	if _, err := configure.SetModel(deps, paths, []string{"hyuga=opus/xhigh"}); err != nil {
		t.Fatalf("SetModel() (add) error = %v", err)
	}

	result, err := configure.SetModel(deps, paths, []string{"hyuga=default"})
	if err != nil {
		t.Fatalf("SetModel() (clear) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "hyuga:") {
		t.Error("cleared role still present")
	}
}

// ---------------------------------------------------------------------------
// setmodel-unknown-role: refused, message names the role.
// ---------------------------------------------------------------------------

func TestSetModel_UnknownRole_Refused(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	_, err := configure.SetModel(deps, paths, []string{"bogus-role=sonnet"})
	if err == nil {
		t.Fatal("SetModel() error = nil, want a refusal")
	}
	var refusal *configure.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	var unknownRole *config.ErrUnknownRole
	if !errors.As(err, &unknownRole) {
		t.Fatalf("error = %v, want to wrap *config.ErrUnknownRole", err)
	}
	if !strings.Contains(err.Error(), "bogus-role") {
		t.Errorf("error message %q does not name the role", err.Error())
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != richFixtureLF {
		t.Error("file changed after a refused --set-model")
	}
}

// ---------------------------------------------------------------------------
// invalid model / effort tokens.
// ---------------------------------------------------------------------------

func TestSetModel_InvalidModelToken_Refused(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	_, err := configure.SetModel(deps, paths, []string{"hyuga=not-a-model"})
	if err == nil {
		t.Fatal("SetModel() error = nil, want a refusal")
	}
	var invalidModel *config.ErrInvalidModel
	if !errors.As(err, &invalidModel) {
		t.Fatalf("error = %v, want to wrap *config.ErrInvalidModel", err)
	}
}

func TestSetModel_InvalidEffortToken_Refused(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	_, err := configure.SetModel(deps, paths, []string{"hyuga=opus/bogus"})
	if err == nil {
		t.Fatal("SetModel() error = nil, want a refusal")
	}
	var invalidEffort *config.ErrInvalidEffort
	if !errors.As(err, &invalidEffort) {
		t.Fatalf("error = %v, want to wrap *config.ErrInvalidEffort", err)
	}
}

func TestSetModel_MalformedEntry_Refused(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	_, err := configure.SetModel(deps, paths, []string{"no-equals-sign"})
	if err == nil {
		t.Fatal("SetModel() error = nil, want a refusal")
	}
	var refusal *configure.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
}

// ---------------------------------------------------------------------------
// from:<phase> display.
// ---------------------------------------------------------------------------

func TestSetModel_FromPhase_Display(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	result, err := configure.SetModel(deps, paths, []string{"melchor=from:jd-judge-a"})
	if err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	want := configure.Change{Key: "models.melchor", From: "from:jd-judge-b", To: "from:jd-judge-a"}
	if len(result.Changes) != 1 || result.Changes[0] != want {
		t.Errorf("Changes = %+v, want [%+v]", result.Changes, want)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "melchor: { from: jd-judge-a }") {
		t.Error("from: override not written")
	}
}

// ---------------------------------------------------------------------------
// group K: setmodel-batch-two-roles — two --set-model entries in one call;
// a no-op entry alongside a real change still reports the real change, and
// a role already at default stays absent from the block.
// ---------------------------------------------------------------------------

func TestSetModel_Batch_TwoRoles(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	result, err := configure.SetModel(deps, paths, []string{"hyuga=opus/xhigh", "rei=default"})
	if err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	want := configure.Change{Key: "models.hyuga", From: "default", To: "opus/xhigh"}
	if len(result.Changes) != 1 || result.Changes[0] != want {
		t.Errorf("Changes = %+v, want [%+v] (rei=default is a no-op)", result.Changes, want)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "hyuga: { model: opus, effort: xhigh }") {
		t.Error("hyuga override not written")
	}
	for _, line := range strings.Split(string(after), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "rei:") {
			t.Errorf("rei should stay absent (still default), found line %q", line)
		}
	}
}

// ---------------------------------------------------------------------------
// from:<phase> validation against gentle-ai's state.json: a phase missing
// from a readable, non-empty claude_phase_assignments map warns (naming the
// phase and the available keys) but the override is still written.
// ---------------------------------------------------------------------------

func TestSetModel_FromPhase_StateWarnings(t *testing.T) {
	const stateWithPhases = `{"claude_phase_assignments":{"jd-judge-b":{"model":"opus","effort":"xhigh"},"sdd-apply":{"model":"sonnet","effort":"high"}}}`

	tests := []struct {
		name        string
		state       string // "" means no state.json on disk
		spec        string
		wantWarning bool
	}{
		{name: "missing phase with readable state warns", state: stateWithPhases, spec: "rei=from:review-risk", wantWarning: true},
		{name: "present phase does not warn", state: stateWithPhases, spec: "rei=from:jd-judge-b", wantWarning: false},
		{name: "absent state does not warn", state: "", spec: "rei=from:review-risk", wantWarning: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "nerv.yaml")
			writeFixture(t, configPath, richFixtureLF)
			statePath := filepath.Join(dir, "state.json")
			if tt.state != "" {
				writeFixture(t, statePath, tt.state)
			}

			deps := newTestDeps(dir, time.Now())
			paths := configure.Paths{Config: configPath, State: statePath}

			result, err := configure.SetModel(deps, paths, []string{tt.spec})
			if err != nil {
				t.Fatalf("SetModel() error = %v", err)
			}

			if tt.wantWarning {
				if len(result.Warnings) != 1 {
					t.Fatalf("Warnings = %q, want exactly one", result.Warnings)
				}
				w := result.Warnings[0]
				for _, want := range []string{"review-risk", "jd-judge-b, sdd-apply"} {
					if !strings.Contains(w, want) {
						t.Errorf("warning %q does not mention %q", w, want)
					}
				}
			} else if len(result.Warnings) != 0 {
				t.Errorf("Warnings = %q, want none", result.Warnings)
			}

			after, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			phase := strings.TrimPrefix(tt.spec, "rei=from:")
			if !strings.Contains(string(after), "rei: { from: "+phase+" }") {
				t.Errorf("from: override for %q not written", phase)
			}
		})
	}
}
