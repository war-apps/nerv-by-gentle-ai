package wizard

import (
	"bytes"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// TestPrintModelTable_LongValuesKeepColumnsAligned proves a custom model id
// and a long gentle-ai source widen their columns instead of shifting the
// rest of the row: the purpose column starts at the same offset on every
// line, header included.
func TestPrintModelTable_LongValuesKeepColumnsAligned(t *testing.T) {
	table := []config.ModelRow{
		{Role: "aoba", Model: "sonnet", Effort: "low", Source: "default"},
		{Role: "kaji-coverage", Model: "claude-sonnet-5-5-20260101", Effort: "medium", Source: "override"},
		{Role: "misato", Model: "fable", Effort: "high", Source: "gentle-ai:sdd-onboard-extended"},
	}
	var out bytes.Buffer
	printModelTable(&out, table, "/tmp/nerv.yaml")

	var rows []string
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.Contains(line, "WHAT IT DOES") || strings.Contains(line, ") ") {
			rows = append(rows, line)
		}
	}
	if len(rows) != 4 {
		t.Fatalf("got %d table lines, want header + 3 rows:\n%s", len(rows), out.String())
	}

	want := strings.Index(rows[0], "WHAT IT DOES")
	purposes := []string{
		config.Roles().Info["aoba"].Purpose,
		config.Roles().Info["kaji-coverage"].Purpose,
		config.Roles().Info["misato"].Purpose,
	}
	for i, purpose := range purposes {
		prefix := purpose
		if len(prefix) > 20 {
			prefix = prefix[:20]
		}
		if got := strings.Index(rows[i+1], prefix); got != want {
			t.Errorf("row %d purpose starts at column %d, want %d (header):\n%s", i+1, got, want, out.String())
		}
	}
}

// TestPrintModelTable_ShowsEveryPurposeInFull proves no catalogue purpose is
// cut off: the column exists to explain each role, so a truncated purpose
// defeats it.
func TestPrintModelTable_ShowsEveryPurposeInFull(t *testing.T) {
	var table []config.ModelRow
	for _, role := range config.Roles().AllRoles {
		table = append(table, config.ModelRow{Role: role, Model: "sonnet", Effort: "medium", Source: "default"})
	}
	var out bytes.Buffer
	printModelTable(&out, table, "/tmp/nerv.yaml")

	for _, role := range config.Roles().AllRoles {
		purpose := config.Roles().Info[role].Purpose
		if !strings.Contains(out.String(), purpose) {
			t.Errorf("role %q purpose %q is not shown in full:\n%s", role, purpose, out.String())
		}
	}
}
