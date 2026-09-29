package release

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

var (
	stableTagPattern     = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	preReleaseTagPattern = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)-rc\.(\d+)$`)
	semverPrefixPattern  = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)
	labelPattern         = regexp.MustCompile(`^[a-z]+$`)
)

type tagCandidate struct {
	tag                    string
	major, minor, patch    int
	releaseRank, preNumber int
}

// LastReleaseTag returns the highest "vX.Y.Z" tag in tags, ordered by
// semantic version (not lexically, so v0.10.0 outranks v0.9.0). Returns ""
// when no matching tag exists. When includePreRelease is true, "vX.Y.Z-rc.N"
// tags are also considered; a stable tag always outranks a pre-release tag
// of the same base version. Ports Get-NervLastReleaseTag (the tag listing
// itself lives in git.go's Tags).
func LastReleaseTag(tags []string, includePreRelease bool) string {
	var candidates []tagCandidate
	for _, t := range tags {
		if t == "" {
			continue
		}
		if m := stableTagPattern.FindStringSubmatch(t); m != nil {
			candidates = append(candidates, tagCandidate{
				tag:         t,
				major:       atoi(m[1]),
				minor:       atoi(m[2]),
				patch:       atoi(m[3]),
				releaseRank: 1,
			})
			continue
		}
		if includePreRelease {
			if m := preReleaseTagPattern.FindStringSubmatch(t); m != nil {
				candidates = append(candidates, tagCandidate{
					tag:         t,
					major:       atoi(m[1]),
					minor:       atoi(m[2]),
					patch:       atoi(m[3]),
					releaseRank: 0,
					preNumber:   atoi(m[4]),
				})
			}
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.major != b.major {
			return a.major > b.major
		}
		if a.minor != b.minor {
			return a.minor > b.minor
		}
		if a.patch != b.patch {
			return a.patch > b.patch
		}
		if a.releaseRank != b.releaseRank {
			return a.releaseRank > b.releaseRank
		}
		return a.preNumber > b.preNumber
	})

	return candidates[0].tag
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// nextPreReleaseNumber is 1 + the highest existing "v<base>-<label>.N" tag
// in existingTags for that exact base version and label (other labels or
// base versions never interfere). Ports Get-NervNextPreReleaseNumber.
func nextPreReleaseNumber(base, label string, existingTags []string) int {
	pattern := regexp.MustCompile(`^v` + regexp.QuoteMeta(base) + `-` + regexp.QuoteMeta(label) + `\.(\d+)$`)
	maxN := 0
	for _, t := range existingTags {
		if t == "" {
			continue
		}
		if m := pattern.FindStringSubmatch(t); m != nil {
			if n := atoi(m[1]); n > maxN {
				maxN = n
			}
		}
	}
	return maxN + 1
}

// NextVersion computes the next semver string from current + bump. Returns
// "" when bump is "none" and preRelease is empty. Ports Get-NervNextVersion.
//
// preReleaseBase selects what preRelease bumps from when preRelease is
// given: "next" (default, empty string also means "next") uses current
// bumped by bump; "current" uses current as-is (never requires a
// releasable bump).
func NextVersion(current, bump, preRelease, preReleaseBase string, existingTags []string) (string, error) {
	if preRelease != "" && !labelPattern.MatchString(preRelease) {
		return "", fmt.Errorf("invalid pre-release label %q; expected lowercase letters only (e.g. 'alpha', 'beta', 'rc')", preRelease)
	}

	if preReleaseBase == "" {
		preReleaseBase = "next"
	}

	if preRelease != "" && preReleaseBase == "current" {
		m := semverPrefixPattern.FindStringSubmatch(current)
		if m == nil {
			return "", fmt.Errorf("current version %q is not valid semver", current)
		}
		base := fmt.Sprintf("%s.%s.%s", m[1], m[2], m[3])
		n := nextPreReleaseNumber(base, preRelease, existingTags)
		return fmt.Sprintf("%s-%s.%d", base, preRelease, n), nil
	}

	if bump == "none" {
		return "", nil
	}

	m := semverPrefixPattern.FindStringSubmatch(current)
	if m == nil {
		return "", fmt.Errorf("current version %q is not valid semver", current)
	}
	major, minor, patch := atoi(m[1]), atoi(m[2]), atoi(m[3])

	switch bump {
	case "major":
		major++
		minor, patch = 0, 0
	case "minor":
		minor++
		patch = 0
	case "patch":
		patch++
	}

	bumpedBase := fmt.Sprintf("%d.%d.%d", major, minor, patch)

	if preRelease != "" {
		n := nextPreReleaseNumber(bumpedBase, preRelease, existingTags)
		return fmt.Sprintf("%s-%s.%d", bumpedBase, preRelease, n), nil
	}

	return bumpedBase, nil
}
