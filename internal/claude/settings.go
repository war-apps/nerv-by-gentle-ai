// Package claude edits Claude Code's global settings.json (marketplace
// registration and plugin enablement), drives the "claude plugin" CLI
// through env.Runner, and reads installed_plugins.json. Mirrors the
// settings-mutation and cache-refresh halves of tools/install.ps1.
package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/war-apps/nerv-gentle-ai/internal/atomicfile"
)

// Settings is Claude Code's settings.json content, held as a generic map
// so every key this package does not know about survives a round-trip
// untouched. Bytes always renders through encoding/json's own
// 2-space-indented map marshaling, which sorts keys alphabetically —
// "preserving ... the file's formatting as much as encoding/json ...
// allows" (see the go-cli feature document): unrelated keys and their
// values survive exactly, only their relative order can shift.
type Settings struct {
	data map[string]any
}

// NewSettings returns an empty Settings, for a settings.json that does not
// exist yet.
func NewSettings() *Settings {
	return &Settings{data: map[string]any{}}
}

// LoadSettings reads and parses path. A missing file reports an error
// wrapping fs.ErrNotExist (checkable with errors.Is); callers that want to
// treat "no settings.json yet" as NewSettings() do so explicitly.
func LoadSettings(path string) (*Settings, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if data == nil {
		data = map[string]any{}
	}
	return &Settings{data: data}, nil
}

// Bytes renders the settings document as indented JSON, terminated with a
// trailing newline.
func (s *Settings) Bytes() ([]byte, error) {
	encoded, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// RegisterMarketplace sets extraKnownMarketplaces.<name>.source to a
// directory marketplace pointing at path, reporting whether the document
// actually changed. Mirrors install.ps1's extraKnownMarketplaces.nerv
// registration.
func (s *Settings) RegisterMarketplace(name, path string) bool {
	marketplaces, _ := s.data["extraKnownMarketplaces"].(map[string]any)
	if marketplaces == nil {
		marketplaces = map[string]any{}
	}

	desired := map[string]any{
		"source": map[string]any{
			"source": "directory",
			"path":   path,
		},
	}

	changed := true
	if existing, ok := marketplaces[name]; ok {
		existingJSON, _ := json.Marshal(existing)
		desiredJSON, _ := json.Marshal(desired)
		changed = string(existingJSON) != string(desiredJSON)
	}

	marketplaces[name] = desired
	s.data["extraKnownMarketplaces"] = marketplaces
	return changed
}

// EnablePlugin sets enabledPlugins[id] = true, reporting whether the
// document actually changed.
func (s *Settings) EnablePlugin(id string) bool {
	enabled, _ := s.data["enabledPlugins"].(map[string]any)
	if enabled == nil {
		enabled = map[string]any{}
	}

	changed := enabled[id] != true
	enabled[id] = true
	s.data["enabledPlugins"] = enabled
	return changed
}

// Unregister removes extraKnownMarketplaces[marketplaceName] and
// enabledPlugins[pluginID] when present, reporting whether either was
// actually removed. Mirrors install.ps1's -Uninstall branch.
func (s *Settings) Unregister(marketplaceName, pluginID string) bool {
	changed := false

	if marketplaces, ok := s.data["extraKnownMarketplaces"].(map[string]any); ok {
		if _, exists := marketplaces[marketplaceName]; exists {
			delete(marketplaces, marketplaceName)
			changed = true
		}
	}

	if enabled, ok := s.data["enabledPlugins"].(map[string]any); ok {
		if _, exists := enabled[pluginID]; exists {
			delete(enabled, pluginID)
			changed = true
		}
	}

	return changed
}

// SaveSettings persists s to path when its rendered bytes differ from
// previous (the bytes an earlier LoadSettings/NewSettings call observed):
// a no-op — no write, no backup — when they are identical. When path
// already exists it is first copied to a timestamped
// "<path>.bak-nerv-<yyyyMMdd-HHmmss>" backup; when it does not exist yet,
// no backup is written. The new content is then written to a temp file in
// the same directory, parse-verified, renamed over path, and re-parsed
// once more to confirm the swap landed cleanly — mirroring install.ps1's
// write-verify-swap sequence. A write or verification failure restores
// the backup (when one was taken) before returning the error.
func SaveSettings(path string, s *Settings, previous []byte, now time.Time) (written bool, backup string, err error) {
	newBytes, err := s.Bytes()
	if err != nil {
		return false, "", err
	}
	if bytes.Equal(newBytes, previous) {
		return false, "", nil
	}

	backup, err = atomicfile.Save(path, newBytes, atomicfile.Options{
		Now:          now,
		BackupSuffix: "bak-nerv-",
		Verify:       verifyJSON,
	})
	if err != nil {
		return false, backup, err
	}
	return true, backup, nil
}

func verifyJSON(data []byte) error {
	var v any
	return json.Unmarshal(data, &v)
}
