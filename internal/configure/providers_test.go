package configure_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
)

// EmbeddedProviders lists every provider plugin nerv ships, by provider
// name, with its parsed manifest.
func TestEmbeddedProviders_ListsTheShippedProviders(t *testing.T) {
	providers, err := configure.EmbeddedProviders(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range providers {
		names = append(names, p.Manifest.Name)
		if p.Plugin != "task-"+p.Manifest.Name {
			t.Errorf("provider %s: plugin %q, want task-%s", p.Manifest.Name, p.Plugin, p.Manifest.Name)
		}
	}
	if !slices.Contains(names, "teamwork") {
		t.Errorf("providers = %v, want teamwork among them", names)
	}
	if none, err := configure.EmbeddedProviders(nil); err != nil || len(none) != 0 {
		t.Errorf("EmbeddedProviders(nil) = %v, %v; want none", none, err)
	}
}

// SetProviderSettings writes unmanaged tasks.* scalars through the
// format-preserving editor: comments and the other keys stay, the line
// ending style is kept, a backup is taken, and a value equal to the current
// one changes nothing.
func TestSetProviderSettings_PreservesFormatAndReportsChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.yml")
	original := "enabled: true\r\n" +
		"tasks:\r\n" +
		"  providers:\r\n" +
		"    teamwork:\r\n" +
		"      assignee_id: 1            # me\r\n" +
		"git:\r\n" +
		"  base_branch: develop\r\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := configure.Deps{Now: func() time.Time { return time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC) }}

	result, err := configure.SetProviderSettings(deps, path, []configure.ProviderSetting{
		{Key: "tasks.providers.teamwork.assignee_id", Value: "686035"},
		{Key: "tasks.providers.teamwork.billable", Value: "true"},
	})
	if err != nil {
		t.Fatalf("SetProviderSettings() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "enabled: true\r\n" +
		"tasks:\r\n" +
		"  providers:\r\n" +
		"    teamwork:\r\n" +
		"      assignee_id: 686035            # me\r\n" +
		"      billable: true\r\n" +
		"git:\r\n" +
		"  base_branch: develop\r\n"
	if string(got) != want {
		t.Errorf("file =\n%q\nwant\n%q", got, want)
	}
	wantChanges := []configure.Change{
		{Key: "tasks.providers.teamwork.assignee_id", From: "1", To: "686035"},
		{Key: "tasks.providers.teamwork.billable", From: "", To: "true"},
	}
	if !result.Changed || !slices.Equal(result.Changes, wantChanges) || result.Backup == nil {
		t.Errorf("result = %+v, want changed with %v and a backup", result, wantChanges)
	}

	again, err := configure.SetProviderSettings(deps, path, []configure.ProviderSetting{{Key: "tasks.providers.teamwork.assignee_id", Value: "686035"}})
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed || len(again.Changes) != 0 {
		t.Errorf("same value: result = %+v, want no change", again)
	}
}

// A user-scope key belongs in the user config.yml, never in a repo
// file, so --repo-provider-config refuses it.
func TestParseProviderConfig_RefusesUserScopeKeys(t *testing.T) {
	_, err := configure.ParseProviderConfig(nerv.ExtensionsFS(), "", []string{"teamwork.assignee_id=5"})
	if err == nil || !strings.Contains(err.Error(), "user") {
		t.Fatalf("expected a scope refusal, got %v", err)
	}
	if _, err := configure.ParseProviderConfig(nerv.ExtensionsFS(), "", []string{"teamwork.project_id=7"}); err != nil {
		t.Fatalf("a repo-scope key was refused: %v", err)
	}
}
