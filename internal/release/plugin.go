package release

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// PluginVersion reads the "version" field out of plugin.json content via a
// JSON parse, the same approach internal/version.PluginVersion uses over an
// fs.FS (this package's callers already hold the content as a string, so it
// parses that string directly instead). The file read itself is the
// caller's job.
func PluginVersion(content string) (string, error) {
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(content), &manifest); err != nil {
		return "", fmt.Errorf("parsing plugin.json content: %w", err)
	}
	if manifest.Version == "" {
		return "", fmt.Errorf(`no "version" field found in plugin.json content`)
	}
	return manifest.Version, nil
}

var pluginVersionWritePattern = regexp.MustCompile(`("version"\s*:\s*")([^"]*)(")`)

// SetPluginVersion rewrites only the `"version": "..."` value inside
// plugin.json content via a targeted regex substitution -- never a JSON
// parse/reserialize -- so every other byte of the file (indentation, key
// order, EOL style, absence of a BOM) survives untouched. changed reports
// whether the output differs from content. Ports Set-NervPluginVersion (the
// file write itself is the caller's job).
func SetPluginVersion(content, version string) (out string, changed bool, err error) {
	if !pluginVersionWritePattern.MatchString(content) {
		return content, false, fmt.Errorf(`no "version" field found in plugin.json content`)
	}
	replaced := false
	out = pluginVersionWritePattern.ReplaceAllStringFunc(content, func(match string) string {
		if replaced {
			return match
		}
		replaced = true
		groups := pluginVersionWritePattern.FindStringSubmatch(match)
		return groups[1] + version + groups[3]
	})
	return out, out != content, nil
}
