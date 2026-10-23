package versioning

import "strings"

// PrependChangelog returns the CHANGELOG content after a release, as
// @semantic-release/changelog 6 writes it (lib/prepare.js): the section
// (Result.ChangelogSection) on top of the current content, under title when
// one is set. current is "" for a new file.
func PrependChangelog(current, section, title string) string {
	current = strings.TrimSpace(current)
	if title != "" && strings.HasPrefix(current, title) {
		current = strings.TrimSpace(current[len(title):])
	}
	content := strings.TrimSpace(section) + "\n"
	if current != "" {
		content += "\n" + current + "\n"
	}
	if title != "" {
		return title + "\n\n" + content
	}
	return content
}
