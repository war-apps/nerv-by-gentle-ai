package shipplan

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is the release nerv ship builds for one project.
type Version struct {
	// Version is the new version; empty when the project has no new
	// release.
	Version string
	// Channel is the release channel, empty for the default channel.
	Channel string
	// Tag is the new release's git tag.
	Tag string
	// Notes are the release notes (Markdown).
	Notes string
	// ChangelogSection is the section the release adds to the CHANGELOG.
	ChangelogSection string
	// LastTag and LastVersion are the previous release on the channel;
	// empty when there is none.
	LastTag     string
	LastVersion string
	// BaseCommit is the origin commit the release is built from.
	BaseCommit string
}

// Releasing reports whether the project has a new release.
func (v Version) Releasing() bool { return v.Version != "" }

// VersionSource gives the next version of a project: an explicit one
// (nerv ship --version), or one computed from the commits since the last
// release.
type VersionSource interface {
	Next(ctx context.Context, project string) (Version, error)
}

// semverRe is a semantic version 2.0.0: no leading zeros in numeric
// identifiers, optional prerelease and build metadata.
var semverRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?` +
	`(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

// ValidVersion reports whether v is a semantic version.
func ValidVersion(v string) bool { return semverRe.MatchString(v) }

// Prerelease is v's prerelease part ("rc.1" of 1.0.0-rc.1), empty for a
// stable version or an invalid one.
func Prerelease(v string) string {
	m := semverRe.FindStringSubmatch(v)
	if m == nil {
		return ""
	}
	return m[4]
}

// CompareVersions orders two semantic versions by semver precedence (build
// metadata ignored): -1, 0 or 1.
func CompareVersions(a, b string) (int, error) {
	ma, mb := semverRe.FindStringSubmatch(a), semverRe.FindStringSubmatch(b)
	for _, m := range []struct {
		v string
		m []string
	}{{a, ma}, {b, mb}} {
		if m.m == nil {
			return 0, fmt.Errorf("%q is not a semantic version", m.v)
		}
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.ParseUint(ma[i], 10, 64)
		y, _ := strconv.ParseUint(mb[i], 10, 64)
		if c := cmp.Compare(x, y); c != 0 {
			return c, nil
		}
	}
	switch pa, pb := ma[4], mb[4]; {
	case pa == pb:
		return 0, nil
	case pa == "":
		return 1, nil
	case pb == "":
		return -1, nil
	default:
		return comparePrerelease(strings.Split(pa, "."), strings.Split(pb, ".")), nil
	}
}

// comparePrerelease compares dot-separated prerelease identifiers:
// numeric ones numerically and below alphanumeric ones, alphanumeric ones
// in ASCII order, a shorter list first when all else is equal.
func comparePrerelease(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		x, errX := strconv.ParseUint(a[i], 10, 64)
		y, errY := strconv.ParseUint(b[i], 10, 64)
		var c int
		switch {
		case errX == nil && errY == nil:
			c = cmp.Compare(x, y)
		case errX == nil:
			c = -1
		case errY == nil:
			c = 1
		default:
			c = strings.Compare(a[i], b[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a), len(b))
}

// TagFor is the tag of version under tagFormat (its ${version} replaced).
func TagFor(tagFormat, version string) string {
	return strings.ReplaceAll(tagFormat, "${version}", version)
}

// VersionFromTag returns the version a tag holds under tagFormat, and
// false when the tag does not follow the format or holds no semantic
// version.
func VersionFromTag(tagFormat, tag string) (string, bool) {
	prefix, suffix, ok := strings.Cut(tagFormat, "${version}")
	if !ok || !strings.HasPrefix(tag, prefix) || !strings.HasSuffix(tag, suffix) || len(tag) < len(prefix)+len(suffix) {
		return "", false
	}
	v := tag[len(prefix) : len(tag)-len(suffix)]
	if !ValidVersion(v) {
		return "", false
	}
	return v, true
}
