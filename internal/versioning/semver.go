package versioning

import (
	"regexp"
	"strconv"
	"strings"
)

// semverRe is node-semver's strict FULL pattern: an optional leading v or =,
// major.minor.patch without leading zeros, optional prerelease and build.
var semverRe = regexp.MustCompile(`^[v=]?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][a-zA-Z0-9-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][a-zA-Z0-9-]*))*))?` +
	`(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

// version is a parsed semantic version.
type version struct {
	major, minor, patch int
	pre                 []string
}

// parseVersion parses a version as node-semver's valid(clean(v)) does:
// surrounding spaces and a leading v or = are ignored, build metadata is
// dropped.
func parseVersion(s string) (version, bool) {
	m := semverRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return version{}, false
	}
	v := version{}
	var err error
	if v.major, err = strconv.Atoi(m[1]); err != nil {
		return version{}, false
	}
	if v.minor, err = strconv.Atoi(m[2]); err != nil {
		return version{}, false
	}
	if v.patch, err = strconv.Atoi(m[3]); err != nil {
		return version{}, false
	}
	if m[4] != "" {
		v.pre = strings.Split(m[4], ".")
	}
	return v, true
}

// validVersion reports whether s is a valid semantic version.
func validVersion(s string) bool {
	_, ok := parseVersion(s)
	return ok
}

func (v version) String() string {
	s := strconv.Itoa(v.major) + "." + strconv.Itoa(v.minor) + "." + strconv.Itoa(v.patch)
	if len(v.pre) > 0 {
		s += "-" + strings.Join(v.pre, ".")
	}
	return s
}

func (v version) isPrerelease() bool { return len(v.pre) > 0 }

// hasPrereleaseID reports whether id is one of the prerelease identifiers.
func (v version) hasPrereleaseID(id string) bool {
	for _, p := range v.pre {
		if p == id {
			return true
		}
	}
	return false
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// compareVersions orders versions as semver 2.0 does: a version without a
// prerelease is higher than the same version with one.
func compareVersions(a, b version) int {
	if c := compareInt(a.major, b.major); c != 0 {
		return c
	}
	if c := compareInt(a.minor, b.minor); c != 0 {
		return c
	}
	if c := compareInt(a.patch, b.patch); c != 0 {
		return c
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; ; i++ {
		switch {
		case i >= len(a.pre) && i >= len(b.pre):
			return 0
		case i >= len(a.pre):
			return -1
		case i >= len(b.pre):
			return 1
		}
		x, y := a.pre[i], b.pre[i]
		if x == y {
			continue
		}
		xn, yn := isNumeric(x), isNumeric(y)
		switch {
		case xn && yn:
			xi, _ := strconv.Atoi(x)
			yi, _ := strconv.Atoi(y)
			return compareInt(xi, yi)
		case xn:
			return -1
		case yn:
			return 1
		case x < y:
			return -1
		default:
			return 1
		}
	}
}

// compareVersionStrings compares two valid version strings.
func compareVersionStrings(a, b string) int {
	va, _ := parseVersion(a)
	vb, _ := parseVersion(b)
	return compareVersions(va, vb)
}

// inc increments v by a release type exactly as node-semver's inc does
// (major, minor, patch or prerelease, without a prerelease identifier).
func (v version) inc(releaseType string) version {
	switch releaseType {
	case "major":
		if v.minor != 0 || v.patch != 0 || len(v.pre) == 0 {
			v.major++
		}
		v.minor, v.patch, v.pre = 0, 0, nil
	case "minor":
		if v.patch != 0 || len(v.pre) == 0 {
			v.minor++
		}
		v.patch, v.pre = 0, nil
	case "patch":
		if len(v.pre) == 0 {
			v.patch++
		}
		v.pre = nil
	case "prerelease":
		if len(v.pre) == 0 {
			v.patch++
			v.pre = []string{"0"}
			return v
		}
		pre := append([]string(nil), v.pre...)
		bumped := false
		for i := len(pre) - 1; i >= 0; i-- {
			if isNumeric(pre[i]) {
				n, _ := strconv.Atoi(pre[i])
				pre[i] = strconv.Itoa(n + 1)
				bumped = true
				break
			}
		}
		if !bumped {
			pre = append(pre, "0")
		}
		v.pre = pre
	}
	return v
}

// incString increments a valid version string; "" when it is not valid.
func incString(s, releaseType string) string {
	v, ok := parseVersion(s)
	if !ok {
		return ""
	}
	return v.inc(releaseType).String()
}

// highestVersion returns the higher of two version strings (either may be
// empty), as semantic-release's highest does.
func highestVersion(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	case compareVersionStrings(a, b) > 0:
		return a
	}
	return b
}
