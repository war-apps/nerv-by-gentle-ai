// Package claude edits Claude Code's global settings.json (marketplace
// registration and plugin enablement), drives the "claude plugin" CLI
// through runner.Runner, and reads installed_plugins.json. It is a thin,
// nerv-named layer over nerv-kit's claudeplugin package: nerv's own backup
// suffix ("bak-nerv-") and plugin id ("nerv@nerv") are supplied here so
// every other package keeps importing this package instead of the kit
// directly.
package claude

import (
	"time"

	"github.com/war-apps/nerv-kit-by-gentle-ai/claudeplugin"
)

// Settings is Claude Code's settings.json content; see
// claudeplugin.Settings for its behavior.
type Settings = claudeplugin.Settings

// NewSettings returns an empty Settings, for a settings.json that does not
// exist yet.
func NewSettings() *Settings {
	return claudeplugin.NewSettings()
}

// LoadSettings reads and parses path. A missing file reports an error
// wrapping fs.ErrNotExist (checkable with errors.Is); callers that want to
// treat "no settings.json yet" as NewSettings() do so explicitly.
func LoadSettings(path string) (*Settings, error) {
	return claudeplugin.LoadSettings(path)
}

// SaveSettings persists s to path when its rendered bytes differ from
// previous (the bytes an earlier LoadSettings/NewSettings call observed):
// a no-op — no write, no backup — when they are identical. When path
// already exists it is first copied to a timestamped
// "<path>.bak-nerv-<yyyyMMdd-HHmmss>" backup, keeping the source file's own
// permission bits; when it does not exist yet, no backup is written. The
// new content is then written to a temp file in the same directory,
// parse-verified, renamed over path, and re-parsed once more to confirm the
// swap landed cleanly. A write or verification failure restores the backup
// (when one was taken) before returning the error.
func SaveSettings(path string, s *Settings, previous []byte, now time.Time) (written bool, backup string, err error) {
	return claudeplugin.SaveSettings(path, s, previous, now, "bak-nerv-")
}
