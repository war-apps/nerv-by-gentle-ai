package release

import (
	"fmt"
	"regexp"
	"strings"
)

// changelogGroups is the fixed Keep-a-Changelog group order used by both
// ChangelogSection (build) and implicitly documented for SectionBody
// (read): Breaking, Added, Changed, Fixed.
var changelogGroupOrder = []string{"Breaking", "Added", "Changed", "Fixed"}

// ChangelogSection builds one Keep-a-Changelog section for a version from
// its commits: "## [X.Y.Z] - YYYY-MM-DD", then only the non-empty groups in
// order "### Breaking", "### Added" (feat), "### Changed" (refactor, perf),
// "### Fixed" (fix). Each line is "- <scope: ><description> (<shortSha>)".
// Commits whose type is docs/chore/test/ci/build/style/revert, or that are
// not Conventional Commits, are omitted (unless breaking, in which case
// they land in Breaking regardless of type). Ports New-NervChangelogSection.
func ChangelogSection(version, date string, commits []Commit) string {
	groups := map[string][]string{}
	for _, name := range changelogGroupOrder {
		groups[name] = nil
	}

	for _, cm := range commits {
		info := ConventionalInfo(cm.Subject, cm.Body)
		var line string
		if info.Scope != "" {
			line = fmt.Sprintf("- %s: %s (%s)", info.Scope, info.Description, cm.ShortSha)
		} else {
			line = fmt.Sprintf("- %s (%s)", info.Description, cm.ShortSha)
		}

		switch {
		case info.Breaking:
			groups["Breaking"] = append(groups["Breaking"], line)
		case !info.Conventional:
			// omitted
		default:
			switch info.Type {
			case "feat":
				groups["Added"] = append(groups["Added"], line)
			case "refactor", "perf":
				groups["Changed"] = append(groups["Changed"], line)
			case "fix":
				groups["Fixed"] = append(groups["Fixed"], line)
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## [%s] - %s", version, date)

	for _, name := range changelogGroupOrder {
		lines := groups[name]
		if len(lines) == 0 {
			continue
		}
		b.WriteString("\n\n### ")
		b.WriteString(name)
		for _, l := range lines {
			b.WriteString("\n")
			b.WriteString(l)
		}
	}
	b.WriteString("\n")

	return b.String()
}

var (
	sectionHeadingPattern   = regexp.MustCompile(`^## \[(?P<v>[^\]]+)\]`)
	unreleasedHeadingRegexp = regexp.MustCompile(`(?m)^## \[Unreleased\][^\r\n]*`)
	nextHeadingRegexp       = regexp.MustCompile(`(?m)^## \[`)

	standardChangelogHeader = "# Changelog\n\n" +
		"All notable changes to this project will be documented in this file.\n\n" +
		"The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),\n" +
		"and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).\n\n" +
		"## [Unreleased]\n"
)

// UpdateChangelog inserts section (as produced by ChangelogSection) right
// after the "## [Unreleased]" heading block of content -- so newest-applied
// sections stay on top, right under Unreleased, above any older version
// sections. content == "" is treated as "the file does not exist yet" (the
// caller passes "" when its read failed with not-exist) and is seeded with
// the standard Keep-a-Changelog header before inserting. inserted is false
// without changing content when a section for the same version already
// exists (idempotent). Ports Update-NervChangelog (the file read/write
// itself is the caller's job).
func UpdateChangelog(content, section string) (out string, inserted bool, err error) {
	m := sectionHeadingPattern.FindStringSubmatch(section)
	if m == nil {
		return content, false, fmt.Errorf("section text must start with a '## [X.Y.Z]' heading")
	}
	version := m[1]
	versionHeadingPattern := regexp.MustCompile(`(?m)^## \[` + regexp.QuoteMeta(version) + `\](\s|$)`)

	text := content
	if text == "" {
		text = standardChangelogHeader
	}

	if versionHeadingPattern.MatchString(text) {
		return content, false, nil
	}

	unreleasedMatch := unreleasedHeadingRegexp.FindStringIndex(text)
	if unreleasedMatch == nil {
		return content, false, fmt.Errorf("no '## [Unreleased]' heading found")
	}
	afterHeadingIndex := unreleasedMatch[1]

	rest := text[afterHeadingIndex:]
	insertIndex := len(text)
	if loc := nextHeadingRegexp.FindStringIndex(rest); loc != nil {
		insertIndex = afterHeadingIndex + loc[0]
	}

	before := strings.TrimRight(text[:insertIndex], "\n")
	after := strings.TrimLeft(text[insertIndex:], "\n")
	sectionBlock := strings.Trim(section, "\n")

	newText := before + "\n\n" + sectionBlock + "\n\n" + after
	if !strings.HasSuffix(newText, "\n") {
		newText += "\n"
	}

	return newText, true, nil
}

// SectionBody extracts the body of one "## [<version>] ..." changelog
// section from changelog: everything after that heading line up to (but not
// including) the next "## [" heading, or end of text when it is the last
// section. Leading and trailing blank lines are trimmed. ok is false when no
// "## [<version>]" heading is found. Ports Get-NervChangelogSectionBody.
func SectionBody(changelog, version string) (body string, ok bool) {
	headingPattern := regexp.MustCompile(`(?m)^## \[` + regexp.QuoteMeta(version) + `\][^\r\n]*`)
	loc := headingPattern.FindStringIndex(changelog)
	if loc == nil {
		return "", false
	}

	rest := changelog[loc[1]:]
	section := rest
	if nextLoc := nextHeadingRegexp.FindStringIndex(rest); nextLoc != nil {
		section = rest[:nextLoc[0]]
	}

	return strings.Trim(section, "\r\n"), true
}
