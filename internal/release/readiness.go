package release

import "fmt"

// Result is a release readiness check outcome. Ok=false with a Reason is a
// normal, expected outcome (callers map that to exit 1); Readiness itself
// never errors -- a git/file access failure is the caller's job to
// translate to exit 2 before calling it.
type Result struct {
	Version          string
	Tag              string
	TagExists        bool
	ChangelogSection bool
	Ok               bool
	Reason           string
	Body             string
}

// Readiness checks whether pluginVersion is ready to release: the tag
// "v<pluginVersion>" must not already be present in tags, and changelog
// must contain a matching "## [<pluginVersion>]" section (changelogExists
// distinguishes a missing CHANGELOG.md from one that exists but lacks the
// section). Ports Test-NervReleaseReadiness (the git tag lookup and the
// CHANGELOG.md existence/read are the caller's job).
func Readiness(pluginVersion string, tags []string, changelogExists bool, changelog string) Result {
	tag := "v" + pluginVersion

	tagExists := false
	for _, t := range tags {
		if t == tag {
			tagExists = true
			break
		}
	}

	var body string
	hasSection := false
	if changelogExists {
		if b, ok := SectionBody(changelog, pluginVersion); ok {
			body = b
			hasSection = true
		}
	}

	ok := true
	reason := ""
	switch {
	case tagExists:
		ok = false
		reason = fmt.Sprintf("tag %q already exists", tag)
	case !changelogExists:
		ok = false
		reason = "CHANGELOG.md not found"
	case !hasSection:
		ok = false
		reason = fmt.Sprintf("no '## [%s]' section found in CHANGELOG.md", pluginVersion)
	}

	return Result{
		Version:          pluginVersion,
		Tag:              tag,
		TagExists:        tagExists,
		ChangelogSection: hasSection,
		Ok:               ok,
		Reason:           reason,
		Body:             body,
	}
}
