package config_test

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// ---------------------------------------------------------------------------
// Case group A: Format-NervModelsBlock (ported as config.FormatModelsBlock)
// ---------------------------------------------------------------------------

func TestFormatModelsBlock_GroupA(t *testing.T) {
	overrides := map[string]config.ModelOverride{
		"misato": {Model: "fable", Effort: "high"},
		"aoba":   {From: "jd-judge-b"},
	}
	block := config.FormatModelsBlock(overrides)
	lines := strings.Split(block, "\n")

	t.Run("format-header-line", func(t *testing.T) {
		want := "models:                             # per-role model and effort (written by nerv configure --set-model)"
		if lines[0] != want {
			t.Errorf("got %q, want %q", lines[0], want)
		}
	})
	t.Run("format-sorted-aoba-first", func(t *testing.T) {
		if !regexpMatch(t, `^\s*aoba: \{ from: jd-judge-b \}$`, lines[1]) {
			t.Errorf("got %q", lines[1])
		}
	})
	t.Run("format-misato-explicit-syntax", func(t *testing.T) {
		if !regexpMatch(t, `^\s*misato: \{ model: fable, effort: high \}$`, lines[2]) {
			t.Errorf("got %q", lines[2])
		}
	})
	t.Run("format-no-trailing-blank-line", func(t *testing.T) {
		if strings.HasSuffix(block, "\n") || len(lines) != 3 {
			t.Errorf("got %d lines, trailing-newline=%v", len(lines), strings.HasSuffix(block, "\n"))
		}
	})
	t.Run("format-uses-lf", func(t *testing.T) {
		if strings.Contains(block, "\r\n") {
			t.Errorf("block contains CRLF")
		}
	})
}

func regexpMatch(t *testing.T, pattern, s string) bool {
	t.Helper()
	return regexp.MustCompile(pattern).MatchString(s)
}

// ---------------------------------------------------------------------------
// Case group B: Set-NervYamlModelsBlock (ported as config.SetModelsBlock)
// ---------------------------------------------------------------------------

func TestSetModelsBlock_GroupB(t *testing.T) {
	yamlWithBlockLf := "skills: {}\n" +
		"models:                             # old header\n" +
		"  aoba: { model: haiku, effort: low }\n" +
		"# balthasar: { model: sonnet, effort: medium }\n" +
		"#   casper: { model: sonnet, effort: medium }\n" +
		"  rei: { from: jd-judge-b }\n" +
		"critical_paths: [auth/, payments/, migrations/, infra/]"
	newBlock := "models:                             # new header\n  misato: { model: fable, effort: high }"

	t.Run("set-replace-exact", func(t *testing.T) {
		doc := config.Parse(yamlWithBlockLf)
		config.SetModelsBlock(doc, newBlock)
		want := "skills: {}\n" + newBlock + "\ncritical_paths: [auth/, payments/, migrations/, infra/]\n"
		if got := doc.String(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("append-ends-with-eol_set-append-when-absent", func(t *testing.T) {
		noBlockYaml := "skills: {}\ncritical_paths: [auth/]"
		doc := config.Parse(noBlockYaml)
		config.SetModelsBlock(doc, newBlock)
		got := doc.String()
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("append-ends-with-eol: got %q", got)
		}
		want := noBlockYaml + "\n\n" + newBlock + "\n"
		if got != want {
			t.Errorf("set-append-when-absent: got %q, want %q", got, want)
		}
	})

	t.Run("replace-ends-with-eol", func(t *testing.T) {
		doc := config.Parse(yamlWithBlockLf)
		config.SetModelsBlock(doc, newBlock)
		if got := doc.String(); !strings.HasSuffix(got, "\n") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("set-remove-when-empty", func(t *testing.T) {
		doc := config.Parse(yamlWithBlockLf)
		config.SetModelsBlock(doc, "")
		want := "skills: {}\ncritical_paths: [auth/, payments/, migrations/, infra/]\n"
		if got := doc.String(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("set-crlf-preserved_set-crlf-content-matches-lf-result", func(t *testing.T) {
		yamlWithBlockCrlf := strings.ReplaceAll(yamlWithBlockLf, "\n", "\r\n")
		doc := config.Parse(yamlWithBlockCrlf)
		config.SetModelsBlock(doc, newBlock)
		got := doc.String()
		if hasBareLF(got) {
			t.Errorf("bare LF found in CRLF document")
		}
		lfDoc := config.Parse(yamlWithBlockLf)
		config.SetModelsBlock(lfDoc, newBlock)
		if strings.ReplaceAll(got, "\r\n", "\n") != lfDoc.String() {
			t.Errorf("content mismatch once normalized to LF")
		}
	})
}

// block-models-delegates-to-existing-function (case group A of
// configure.test.ps1): the generic Document.SetBlock("models", ...) must
// match config.SetModelsBlock exactly.
func TestSetModelsBlock_DelegatesToGenericSetBlock(t *testing.T) {
	newModelsBlock := "models:\n  aoba: { model: haiku, effort: low }"
	docGeneric := config.Parse(fixtureLf)
	docGeneric.SetBlock("models", newModelsBlock)
	docModels := config.Parse(fixtureLf)
	config.SetModelsBlock(docModels, newModelsBlock)
	if docGeneric.String() != docModels.String() {
		t.Errorf("SetBlock(\"models\", ...) diverged from SetModelsBlock:\n%s\n---\n%s", docGeneric.String(), docModels.String())
	}
}

// ---------------------------------------------------------------------------
// Case group C: Get-NervModelTable (ported as config.ModelTable)
// ---------------------------------------------------------------------------

func TestModelTable_GroupC(t *testing.T) {
	defaults := map[string]config.ModelOverride{
		"aoba":   {Model: "sonnet", Effort: "low"},
		"rei":    {Model: "sonnet", Effort: "medium"},
		"misato": {Model: "fable", Effort: "high"},
	}
	overrides := map[string]config.ModelOverride{
		"aoba": {Model: "haiku", Effort: "low"},
		"rei":  {From: "jd-judge-b"},
	}
	phaseAssignments := map[string]config.PhaseAssignment{
		"jd-judge-b": {Model: "opus", Effort: "xhigh"},
	}

	rows := config.ModelTable(defaults, overrides, phaseAssignments)
	byRole := map[string]config.ModelRow{}
	for _, r := range rows {
		byRole[r.Role] = r
	}

	t.Run("table-override-source", func(t *testing.T) {
		row, ok := byRole["aoba"]
		if !ok || row.Model != "haiku" || row.Effort != "low" || row.Source != "override" {
			t.Errorf("got %+v, ok=%v", row, ok)
		}
	})
	t.Run("table-gentle-ai-from-source", func(t *testing.T) {
		row, ok := byRole["rei"]
		if !ok || row.Model != "opus" || row.Effort != "xhigh" || row.Source != "gentle-ai:jd-judge-b" {
			t.Errorf("got %+v, ok=%v", row, ok)
		}
	})
	t.Run("table-default-source", func(t *testing.T) {
		row, ok := byRole["misato"]
		if !ok || row.Model != "fable" || row.Effort != "high" || row.Source != "default" {
			t.Errorf("got %+v, ok=%v", row, ok)
		}
	})
}

func TestModelTable_FromPhaseResolution(t *testing.T) {
	defaults := map[string]config.ModelOverride{
		"rei": {Model: "sonnet", Effort: "medium"},
	}

	tests := []struct {
		name             string
		from             string
		phaseAssignments map[string]config.PhaseAssignment
		want             config.ModelRow
	}{
		{
			name:             "missing phase with known assignments is flagged and falls back to plugin default",
			from:             "review-risk",
			phaseAssignments: map[string]config.PhaseAssignment{"jd-judge-b": {Model: "opus", Effort: "xhigh"}},
			want:             config.ModelRow{Role: "rei", Model: "sonnet", Effort: "medium", Source: "gentle-ai:review-risk (missing; plugin default)"},
		},
		{
			name:             "present phase resolves to its assignment",
			from:             "jd-judge-b",
			phaseAssignments: map[string]config.PhaseAssignment{"jd-judge-b": {Model: "opus", Effort: "xhigh"}},
			want:             config.ModelRow{Role: "rei", Model: "opus", Effort: "xhigh", Source: "gentle-ai:jd-judge-b"},
		},
		{
			name:             "unknown assignments (nil) keep the unflagged source",
			from:             "review-risk",
			phaseAssignments: nil,
			want:             config.ModelRow{Role: "rei", Model: "sonnet", Effort: "medium", Source: "gentle-ai:review-risk"},
		},
		{
			name:             "unknown assignments (absent state, empty map) keep the unflagged source",
			from:             "review-risk",
			phaseAssignments: map[string]config.PhaseAssignment{},
			want:             config.ModelRow{Role: "rei", Model: "sonnet", Effort: "medium", Source: "gentle-ai:review-risk"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrides := map[string]config.ModelOverride{"rei": {From: tt.from}}
			rows := config.ModelTable(defaults, overrides, tt.phaseAssignments)
			if len(rows) != 1 || rows[0] != tt.want {
				t.Errorf("got %+v, want [%+v]", rows, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ReadModelsOverrides — the raw scan half of Read-NervModelsOverrides (the
// cross-check against gentle-ai's resolved assignments needs the resolved
// assignment table and is out of this package's scope; see the doc
// comment). Not its own PS unit case group, so covered here directly.
// ---------------------------------------------------------------------------

func TestReadModelsOverrides(t *testing.T) {
	doc := config.Parse(fixtureLf)
	overrides := config.ReadModelsOverrides(doc)

	t.Run("parses-model-and-effort", func(t *testing.T) {
		got, ok := overrides["misato"]
		if !ok || got.Model != "fable" || got.Effort != "high" {
			t.Errorf("got %+v, ok=%v", got, ok)
		}
	})
	t.Run("parses-from", func(t *testing.T) {
		got, ok := overrides["melchior"]
		if !ok || got.From != "jd-judge-b" {
			t.Errorf("got %+v, ok=%v", got, ok)
		}
	})
	t.Run("no-models-block-returns-empty-map", func(t *testing.T) {
		empty := config.ReadModelsOverrides(config.Parse("enabled: true\n"))
		if len(empty) != 0 {
			t.Errorf("got %v", empty)
		}
	})
}

// A nerv.yaml written before the melchor -> melchior and kaji-audit -> gendo
// renames keeps applying: legacy keys resolve to the new role ID, and when
// both the legacy and the new key are present the new key wins, whatever
// their order in the file.
func TestReadModelsOverrides_LegacyAliases(t *testing.T) {
	cases := []struct {
		name  string
		block string
		want  map[string]config.ModelOverride
	}{
		{
			name:  "legacy-keys-resolve-to-new-ids",
			block: "models:\n  melchor: { from: jd-judge-b }\n  kaji-audit: { model: opus, effort: high }\n",
			want: map[string]config.ModelOverride{
				"melchior": {From: "jd-judge-b"},
				"gendo":    {Model: "opus", Effort: "high"},
			},
		},
		{
			name:  "new-key-after-legacy-wins",
			block: "models:\n  melchor: { model: haiku }\n  melchior: { model: opus }\n",
			want:  map[string]config.ModelOverride{"melchior": {Model: "opus"}},
		},
		{
			name:  "new-key-before-legacy-wins",
			block: "models:\n  gendo: { model: opus }\n  kaji-audit: { model: haiku, effort: low }\n",
			want:  map[string]config.ModelOverride{"gendo": {Model: "opus"}},
		},
		{
			name:  "unknown-keys-kept-as-written",
			block: "models:\n  retired-role: { model: opus }\n",
			want:  map[string]config.ModelOverride{"retired-role": {Model: "opus"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ReadModelsOverrides(config.Parse("enabled: true\n" + tc.block))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ModelTable, fed from ReadModelsOverrides, shows a legacy override on the
// renamed role's row and adds no row for the legacy name.
func TestModelTableFromDocument_LegacyAliasAppliesToRenamedRole(t *testing.T) {
	doc := config.Parse("models:\n  kaji-audit: { model: opus, effort: xhigh }\n")
	defaults := map[string]config.ModelOverride{
		"gendo":    {Model: "sonnet", Effort: "medium"},
		"melchior": {Model: "fable", Effort: "high"},
	}
	rows := config.ModelTableFromDocument(doc, defaults, nil)
	want := []config.ModelRow{
		{Role: "gendo", Model: "opus", Effort: "xhigh", Source: "override"},
		{Role: "melchior", Model: "fable", Effort: "high", Source: "default"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("got %+v, want %+v", rows, want)
	}
}

// The user and project models: blocks are layered by the orchestrator (no Go
// code merges the two scopes; apply-models reads the user scope only). The
// layering is correct only if each scope is canonicalised before the project
// entry overrides the user entry role by role, so a legacy key in one scope
// and the new key in the other meet under the same role ID. These cases pin
// that ReadModelsOverrides delivers those per-scope canonical maps.
func TestReadModelsOverrides_LayeredScopesMeetUnderCanonicalIDs(t *testing.T) {
	cases := []struct {
		name          string
		user, project string
		want          map[string]config.ModelOverride
	}{
		{
			name:    "project-legacy-key-overrides-user-new-key",
			user:    "models:\n  melchior: { model: opus }\n",
			project: "models:\n  melchor: { model: sonnet }\n",
			want:    map[string]config.ModelOverride{"melchior": {Model: "sonnet"}},
		},
		{
			name:    "project-new-key-overrides-user-legacy-key",
			user:    "models:\n  kaji-audit: { model: opus, effort: high }\n",
			project: "models:\n  gendo: { model: sonnet }\n",
			want:    map[string]config.ModelOverride{"gendo": {Model: "sonnet"}},
		},
		{
			name:    "user-legacy-key-applies-when-project-is-silent",
			user:    "models:\n  melchor: { model: haiku }\n",
			project: "models:\n  aoba: { model: sonnet }\n",
			want: map[string]config.ModelOverride{
				"melchior": {Model: "haiku"},
				"aoba":     {Model: "sonnet"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ReadModelsOverrides(config.Parse(tc.user))
			for role, entry := range config.ReadModelsOverrides(config.Parse(tc.project)) {
				got[role] = entry
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("layered overrides = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Role catalogue (Get-NervRoleCatalogue). No direct PS unit-test case group
// exists for this function in tests/configure-models.test.ps1 (it is only
// exercised indirectly, through the interactive e2e answer files). Coverage
// below is derived directly from the function's own implementation.
// ---------------------------------------------------------------------------

func TestRoles(t *testing.T) {
	catalogue := config.Roles()

	t.Run("16-roles", func(t *testing.T) {
		if len(catalogue.AllRoles) != 16 {
			t.Errorf("got %d roles", len(catalogue.AllRoles))
		}
	})
	t.Run("magi-group", func(t *testing.T) {
		want := []string{"balthasar", "melchior", "casper"}
		if strings.Join(catalogue.Groups["magi"], ",") != strings.Join(want, ",") {
			t.Errorf("got %v", catalogue.Groups["magi"])
		}
	})
	t.Run("pilots-group", func(t *testing.T) {
		want := []string{"rei", "shinji", "asuka", "toji", "kaworu"}
		if strings.Join(catalogue.Groups["pilots"], ",") != strings.Join(want, ",") {
			t.Errorf("got %v", catalogue.Groups["pilots"])
		}
	})
	t.Run("audit-passes-group", func(t *testing.T) {
		want := []string{"kaji", "gendo"}
		if strings.Join(catalogue.Groups["audit-passes"], ",") != strings.Join(want, ",") {
			t.Errorf("got %v", catalogue.Groups["audit-passes"])
		}
	})
	t.Run("all-group-equals-all-roles", func(t *testing.T) {
		if strings.Join(catalogue.Groups["all"], ",") != strings.Join(catalogue.AllRoles, ",") {
			t.Errorf("got %v", catalogue.Groups["all"])
		}
	})
}

// ---------------------------------------------------------------------------
// ResolveRoleTarget (Resolve-NervRoleTarget) — the interactive wizard's
// role/group/number selection resolver. No direct PS unit-test case group
// either (same caveat as Get-NervRoleCatalogue above).
// ---------------------------------------------------------------------------

func TestResolveRoleTarget(t *testing.T) {
	catalogue := config.Roles()
	numberMap := map[string]string{"1": "aoba", "2": "asuka"}

	cases := []struct {
		name   string
		target string
		want   []string
	}{
		{"empty", "", nil},
		{"group-lowercase", "magi", []string{"balthasar", "melchior", "casper"}},
		{"group-case-insensitive", "MAGI", []string{"balthasar", "melchior", "casper"}},
		{"number", "1", []string{"aoba"}},
		{"role-name", "aoba", []string{"aoba"}},
		{"role-name-case-insensitive", "AOBA", []string{"aoba"}},
		{"audit-passes-group", "audit-passes", []string{"kaji", "gendo"}},
		{"legacy-group-alias", "kaji-passes", []string{"kaji", "gendo"}},
		{"legacy-group-alias-case-insensitive", "KAJI-PASSES", []string{"kaji", "gendo"}},
		{"legacy-role-alias-melchor", "melchor", []string{"melchior"}},
		{"legacy-role-alias-kaji-audit", "Kaji-Audit", []string{"gendo"}},
		{"unknown", "not-a-role", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ResolveRoleTarget(tc.target, catalogue.AllRoles, catalogue.Groups, numberMap)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ResolveModelSpec — the -SetModel per-entry "role=<spec>" parser/validator
// inlined in configure.ps1's non-interactive body (not its own named
// function there, so there is no PS suite case group for it either; ported
// from the inline logic directly). Error wording matches that script's
// Write-Host text exactly.
// ---------------------------------------------------------------------------

func TestResolveModelSpec(t *testing.T) {
	t.Run("default-clears", func(t *testing.T) {
		override, clear, err := config.ResolveModelSpec("misato", "default")
		if err != nil || !clear || override != (config.ModelOverride{}) {
			t.Errorf("got %+v, clear=%v, err=%v", override, clear, err)
		}
	})
	t.Run("default-case-insensitive", func(t *testing.T) {
		_, clear, err := config.ResolveModelSpec("misato", "DEFAULT")
		if err != nil || !clear {
			t.Errorf("clear=%v, err=%v", clear, err)
		}
	})
	t.Run("from-phase", func(t *testing.T) {
		override, clear, err := config.ResolveModelSpec("melchior", "from:jd-judge-b")
		if err != nil || clear || override.From != "jd-judge-b" {
			t.Errorf("got %+v, clear=%v, err=%v", override, clear, err)
		}
	})
	t.Run("model-only", func(t *testing.T) {
		override, _, err := config.ResolveModelSpec("misato", "fable")
		if err != nil || override.Model != "fable" || override.Effort != "" {
			t.Errorf("got %+v, err=%v", override, err)
		}
	})
	t.Run("model-with-effort", func(t *testing.T) {
		override, _, err := config.ResolveModelSpec("misato", "fable/high")
		if err != nil || override.Model != "fable" || override.Effort != "high" {
			t.Errorf("got %+v, err=%v", override, err)
		}
	})
	t.Run("inherit-is-a-valid-model", func(t *testing.T) {
		override, _, err := config.ResolveModelSpec("misato", "inherit")
		if err != nil || override.Model != "inherit" {
			t.Errorf("got %+v, err=%v", override, err)
		}
	})
	t.Run("custom-claude-id", func(t *testing.T) {
		override, _, err := config.ResolveModelSpec("misato", "claude-opus-4-1-20250805")
		if err != nil || override.Model != "claude-opus-4-1-20250805" {
			t.Errorf("got %+v, err=%v", override, err)
		}
	})
	t.Run("invalid-model", func(t *testing.T) {
		_, _, err := config.ResolveModelSpec("misato", "gpt5")
		if err == nil {
			t.Fatalf("expected error")
		}
		want := "Invalid model 'gpt5' for role 'misato'. Allowed: sonnet, opus, haiku, fable, inherit, a claude-... id, from:<phase>, or default."
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
		var invalidModelErr *config.ErrInvalidModel
		if !errors.As(err, &invalidModelErr) {
			t.Errorf("expected *config.ErrInvalidModel, got %T", err)
		}
	})
	t.Run("invalid-effort", func(t *testing.T) {
		_, _, err := config.ResolveModelSpec("misato", "fable/ultra")
		if err == nil {
			t.Fatalf("expected error")
		}
		want := "Invalid effort 'ultra' for role 'misato'. Allowed: low, medium, high, xhigh, max."
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
		var invalidEffortErr *config.ErrInvalidEffort
		if !errors.As(err, &invalidEffortErr) {
			t.Errorf("expected *config.ErrInvalidEffort, got %T", err)
		}
	})
}

func TestValidateRole(t *testing.T) {
	t.Run("known-role", func(t *testing.T) {
		if err := config.ValidateRole("misato"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("unknown-role", func(t *testing.T) {
		err := config.ValidateRole("not-a-role")
		if err == nil {
			t.Fatalf("expected error")
		}
		want := "Unknown role 'not-a-role'. Valid roles: " + strings.Join(config.Roles().AllRoles, ", ") + "."
		if err.Error() != want {
			t.Errorf("got %q, want %q", err.Error(), want)
		}
		var unknownRoleErr *config.ErrUnknownRole
		if !errors.As(err, &unknownRoleErr) {
			t.Errorf("expected *config.ErrUnknownRole, got %T", err)
		}
	})
}
