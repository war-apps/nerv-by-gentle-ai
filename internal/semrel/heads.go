package semrel

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// originHeadsRef is where the mirror's fetch puts origin's branches before
// they replace the mirror's heads.
const originHeadsRef = "refs/nerv-origin-heads/"

// remoteTrackingRef holds the user's last-fetched view of origin's branches,
// copied into the mirror by the clone.
const remoteTrackingRef = "refs/remotes/origin/"

// planHeads returns the `git update-ref --stdin` transaction that gives the
// mirror the branches semantic-release must see: origin's, as in CI, never
// the developer's local-only ones (a local hotfix/x matching a
// `hotfix/*` prerelease: true entry made the whole run fail with
// EPRERELEASEBRANCH), plus the requested branch at the user's local commit
// (its unpushed commits count).
//
// refs maps ref names to object ids. source is the prefix of origin's
// branches in refs: originHeadsRef when they were just fetched
// (authoritative: an empty set means origin has no branches),
// remoteTrackingRef offline (used when it holds at least one branch), ""
// when there is no origin. Without a usable source the local branches are
// kept. Every originHeadsRef ref is deleted. Lines are sorted by ref name;
// the empty string means nothing changes.
func planHeads(refs map[string]string, source string, authoritative bool, branch, localSHA string) string {
	current := map[string]string{}
	fromOrigin := map[string]string{}
	var scratch []string
	for name, sha := range refs {
		switch {
		case strings.HasPrefix(name, "refs/heads/"):
			current[name] = sha
		case source != "" && strings.HasPrefix(name, source):
			if b := strings.TrimPrefix(name, source); b != "HEAD" {
				fromOrigin["refs/heads/"+b] = sha
			}
		}
		if strings.HasPrefix(name, originHeadsRef) {
			scratch = append(scratch, name)
		}
	}
	desired := current
	if source != "" && (authoritative || len(fromOrigin) > 0) {
		desired = fromOrigin
	}
	desired = maps.Clone(desired)
	desired["refs/heads/"+branch] = localSHA

	var b strings.Builder
	names := slices.Sorted(maps.Keys(current))
	for name := range desired {
		if _, ok := current[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		want, keep := desired[name]
		have, exists := current[name]
		switch {
		case !keep:
			fmt.Fprintf(&b, "delete %s\n", name)
		case !exists:
			fmt.Fprintf(&b, "create %s %s\n", name, want)
		case have != want:
			fmt.Fprintf(&b, "update %s %s\n", name, want)
		}
	}
	slices.Sort(scratch)
	for _, name := range scratch {
		fmt.Fprintf(&b, "delete %s\n", name)
	}
	return b.String()
}

// setMirrorHeads rewrites the mirror's branches with planHeads, in one
// transaction, on the mirror only.
func (e *Engine) setMirrorHeads(ctx context.Context, mirror string, remote origin, env []string, branch, localSHA string) error {
	source, authoritative := "", false
	switch {
	case remote.URL != "":
		source, authoritative = originHeadsRef, true
	case remote.Exists:
		source = remoteTrackingRef
	}
	out, err := e.git(ctx, mirror, env, "for-each-ref", "--format=%(objectname) %(refname)", "refs/heads/", remoteTrackingRef, originHeadsRef)
	if err != nil {
		return fmt.Errorf("listing the copy's branches: %w", err)
	}
	refs := map[string]string{}
	for line := range strings.Lines(out) {
		if sha, name, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			refs[name] = sha
		}
	}
	plan := planHeads(refs, source, authoritative, branch, localSHA)
	if plan == "" {
		return nil
	}
	if _, err := e.run(ctx, Command{Name: "git", Args: []string{"-C", mirror, "update-ref", "--stdin"}, Env: env, Stdin: []byte(plan)}); err != nil {
		return fmt.Errorf("setting the copy's branches to origin's: %w", err)
	}
	return nil
}
