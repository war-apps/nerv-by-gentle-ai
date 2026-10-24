package versioning

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// This file ports semantic-release 24.2's lib/branches (expand.js,
// normalize.js and index.js's validation) and lib/definitions/branches.js.

// maintenanceRangeRe is semantic-release's isMaintenanceRange.
var maintenanceRangeRe = regexp.MustCompile(`(?i)^\d+(?:\.(?:\d+|x))?\.x$`)

// Branch types.
const (
	branchRelease    = "release"
	branchPrerelease = "prerelease"
)

// resolvedBranch is one configured branch expanded against the repository's
// branches and normalized.
type resolvedBranch struct {
	name       string
	kind       string // branchRelease or branchPrerelease
	prerelease string // the prerelease identifier
	channel    string // "" is the default channel
}

// expandBranches matches every configured entry against the branch names
// (each name goes to the first entry matching it, as expand.js removes it
// from the list), substitutes ${name} in the prerelease identifier and the
// channel, and normalizes and validates the result as semantic-release
// does. Maintenance branches are refused: the native engine does not
// support them.
func expandBranches(entries []Branch, names []string) ([]resolvedBranch, error) {
	pool := slices.Clone(names)
	type expanded struct {
		name       string
		prerelease Prerelease
		channel    string
	}
	var all []expanded
	for _, e := range entries {
		var kept []string
		for _, n := range pool {
			if globMatch(n, e.Name) {
				pre := e.Prerelease
				if !pre.useName {
					pre.id = strings.ReplaceAll(pre.id, "${name}", n)
				}
				all = append(all, expanded{name: n, prerelease: pre, channel: strings.ReplaceAll(e.Channel, "${name}", n)})
			} else {
				kept = append(kept, n)
			}
		}
		pool = kept
	}

	var problems []string
	var releases, prereleases []resolvedBranch
	for _, b := range all {
		switch {
		case !b.prerelease.IsSet() && maintenanceRangeRe.MatchString(b.name):
			problems = append(problems, fmt.Sprintf("branch %s is a maintenance branch, which the version engine does not support", b.name))
		case b.prerelease.IsSet():
			id := b.prerelease.resolve(b.name)
			if !validVersion("1.0.0-" + id + ".1") {
				problems = append(problems, fmt.Sprintf("EPRERELEASEBRANCH: the prerelease branch %s has an invalid prerelease identifier %q (a valid semver prerelease identifier is required; set a fixed one, such as beta)", b.name, id))
			}
			channel := b.channel
			if channel == "" {
				channel = b.name
			}
			prereleases = append(prereleases, resolvedBranch{name: b.name, kind: branchPrerelease, prerelease: id, channel: channel})
		default:
			channel := b.channel
			if len(releases) > 0 && channel == "" {
				channel = b.name
			}
			releases = append(releases, resolvedBranch{name: b.name, kind: branchRelease, channel: channel})
		}
	}
	if len(releases) == 0 || len(releases) > 3 {
		var configured []string
		for _, b := range releases {
			configured = append(configured, b.name)
		}
		problems = append(problems, fmt.Sprintf("ERELEASEBRANCHES: semantic-release needs between 1 and 3 release branches, found %d (%s)", len(releases), strings.Join(configured, ", ")))
	}
	ids := map[string][]string{}
	for _, b := range prereleases {
		ids[b.prerelease] = append(ids[b.prerelease], b.name)
	}
	for _, b := range prereleases {
		if names := ids[b.prerelease]; len(names) > 1 && names[0] == b.name {
			problems = append(problems, fmt.Sprintf("EPRERELEASEBRANCHES: the branches %s share the prerelease identifier %q; each prerelease branch needs its own", strings.Join(names, ", "), b.prerelease))
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("release branches: %s", strings.Join(problems, "; "))
	}
	return append(releases, prereleases...), nil
}

// branchNames lists the configured entry names, for messages.
func branchNames(entries []Branch) string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return strings.Join(out, ", ")
}
