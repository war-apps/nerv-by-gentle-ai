package release_test

import (
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/release"
)

func TestPluginVersion(t *testing.T) {
	t.Run("reads-version", func(t *testing.T) {
		got, err := release.PluginVersion(`{"name":"nerv","version":"1.2.3"}`)
		if err != nil {
			t.Fatalf("PluginVersion() error = %v", err)
		}
		if got != "1.2.3" {
			t.Errorf("PluginVersion() = %q, want 1.2.3", got)
		}
	})

	t.Run("no-version-field-errors", func(t *testing.T) {
		if _, err := release.PluginVersion(`{"name":"nerv"}`); err == nil {
			t.Fatal("expected an error for a missing version field")
		}
	})
}

func TestSetPluginVersion(t *testing.T) {
	before := "{\n  \"name\": \"nerv-release-fixture\",\n  \"version\": \"0.1.0\",\n  \"description\": \"x\",\n  \"author\": { \"name\": \"Test\" }\n}\n"

	t.Run("changes-only-the-version-value", func(t *testing.T) {
		after, changed, err := release.SetPluginVersion(before, "0.2.0")
		if err != nil {
			t.Fatalf("SetPluginVersion() error = %v", err)
		}
		if !changed {
			t.Error("changed = false, want true")
		}

		beforeLines := strings.Split(before, "\n")
		afterLines := strings.Split(after, "\n")
		if len(beforeLines) != len(afterLines) {
			t.Fatalf("line count changed: before=%d after=%d", len(beforeLines), len(afterLines))
		}

		diffCount := 0
		diffIdx := -1
		for i := range beforeLines {
			if beforeLines[i] != afterLines[i] {
				diffCount++
				diffIdx = i
			}
		}
		if diffCount != 1 {
			t.Fatalf("expected exactly one differing line, got %d", diffCount)
		}
		if !strings.Contains(afterLines[diffIdx], `"version": "0.2.0"`) {
			t.Errorf("changed line = %q, want it to contain the new version", afterLines[diffIdx])
		}
	})

	t.Run("no-bom-lf-only-trailing-newline", func(t *testing.T) {
		after, _, err := release.SetPluginVersion(before, "0.2.0")
		if err != nil {
			t.Fatalf("SetPluginVersion() error = %v", err)
		}
		if len(after) > 0 && after[0] == 0xEF {
			t.Error("output starts with a BOM")
		}
		if strings.Contains(after, "\r") {
			t.Error("output contains a CR")
		}
		if !strings.HasSuffix(after, "\n") {
			t.Error("output does not end with a newline")
		}
	})

	t.Run("same-version-still-reports-changed-by-byte-comparison", func(t *testing.T) {
		after, changed, err := release.SetPluginVersion(before, "0.1.0")
		if err != nil {
			t.Fatalf("SetPluginVersion() error = %v", err)
		}
		if after != before {
			t.Errorf("content changed unexpectedly")
		}
		if changed {
			t.Error("changed = true for an identical version, want false")
		}
	})

	t.Run("missing-version-field-errors", func(t *testing.T) {
		_, _, err := release.SetPluginVersion(`{"name":"nerv"}`, "1.0.0")
		if err == nil {
			t.Fatal("expected an error for a missing version field")
		}
	})
}
