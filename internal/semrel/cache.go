package semrel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The plugins installed next to semantic-release in the NERV cache, at exact
// versions verified on npm (2026-10-13). They are the majors that work with
// semantic-release 24 and its bundled commit-analyzer 13 and
// release-notes-generator 14.
//
// conventional-changelog-conventionalcommits and the changelog writer, the
// one statement of record (docs/release-commands.md and the feature log refer
// here): semantic-release 24.2.3 installs release-notes-generator 14.1.1 with
// conventional-changelog-writer 8.4.0. With it, conventionalcommits 8.0.0,
// 9.0.0 and 9.3.1 render notes; 10.0.0 and later fail in generateNotes with
// `Missing helper: "conventional-changelog-conventionalcommits requires
// conventional-changelog-writer@9 or newer ..."` (observed 2026-10-13; 10.x
// also needs Node >= 22). The pin stays on 8.0.0; a 9.x pin through
// release.semantic_release.extra_plugins works too.
//
// @semantic-release/changelog 7 and git 11 also exist but need Node
// ^22.22.2 || >=24.15; the pinned majors run on any Node semantic-release 24
// supports. A release.semantic_release.extra_plugins entry of the same
// package overrides its pin.
const (
	ChangelogVersion           = "6.0.3"
	GitVersion                 = "10.0.1"
	ExecVersion                = "7.1.0"
	ConventionalCommitsVersion = "8.0.0"
)

// defaultPlugins are the pinned packages installed besides semantic-release.
var defaultPlugins = map[string]string{
	"@semantic-release/changelog":                ChangelogVersion,
	"@semantic-release/git":                      GitVersion,
	"@semantic-release/exec":                     ExecVersion,
	"conventional-changelog-conventionalcommits": ConventionalCommitsVersion,
}

// bundledPackages come with semantic-release itself (its dependencies), so a
// configuration may name them without pinning them.
var bundledPackages = []string{
	"@semantic-release/commit-analyzer",
	"@semantic-release/release-notes-generator",
	"@semantic-release/npm",
	"@semantic-release/github",
	"conventional-changelog-angular",
}

// readyMarker is written into a cache directory once its install finished.
const readyMarker = ".nerv-ready"

// installPrefix names the private staging directories of cache installs.
const installPrefix = ".install-"

// staleInstallAge is the age after which a staging directory is taken as
// left behind by a killed run (its deferred removal never ran) and swept. An
// install takes seconds to minutes, so a younger one may still be running.
const staleInstallAge = time.Hour

// splitSpec splits "name@version" (the name may be scoped: "@a/b@1.0.0").
func splitSpec(spec string) (name, version string) {
	at := strings.LastIndex(spec, "@")
	if at <= 0 {
		return spec, ""
	}
	return spec[:at], spec[at+1:]
}

// pinnedSpecs returns the npm specs installed in the cache: semantic-release
// at srVersion, the default plugins and the extra plugins (which override a
// default pin of the same package), sorted.
func pinnedSpecs(srVersion string, extras []string) []string {
	byName := map[string]string{"semantic-release": "semantic-release@" + srVersion}
	for name, version := range defaultPlugins {
		byName[name] = name + "@" + version
	}
	for _, spec := range extras {
		name, _ := splitSpec(spec)
		byName[name] = spec
	}
	specs := make([]string, 0, len(byName))
	for _, spec := range byName {
		specs = append(specs, spec)
	}
	slices.Sort(specs)
	return specs
}

// cacheKey identifies an installed spec set: 16 hex characters of the
// SHA-256 of the sorted specs.
func cacheKey(specs []string) string {
	sorted := slices.Clone(specs)
	slices.Sort(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])[:16]
}

// defaultCacheBase is <user cache dir>/nerv/semantic-release.
func defaultCacheBase() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("semantic-release cache: %w", err)
	}
	return filepath.Join(dir, "nerv", "semantic-release"), nil
}

// ensureCache returns <base>/<cacheKey(specs)>, installing specs there first
// when it is not ready. The install goes to a private temporary directory
// that is renamed into place, so concurrent runs never see a half-installed
// cache: the first rename wins and the others discard their copy.
func (e *Engine) ensureCache(ctx context.Context, base string, specs []string) (string, error) {
	dir := filepath.Join(base, cacheKey(specs))
	if ready(dir) {
		return dir, nil
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmt.Errorf("semantic-release cache: %w", err)
	}
	sweepStaleInstalls(base, time.Now().Add(-staleInstallAge))
	tmp, err := os.MkdirTemp(base, installPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("semantic-release cache: %w", err)
	}
	defer os.RemoveAll(tmp)

	args := append([]string{"install", "--no-audit", "--no-fund", "--ignore-scripts", "--loglevel=error", "--prefix", tmp}, specs...)
	if _, err := e.run(ctx, Command{Name: "npm", Args: args, Dir: tmp}); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", errors.New("npm is required to install semantic-release (install Node.js, which includes npm)")
		}
		return "", fmt.Errorf("installing semantic-release into the NERV cache: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, readyMarker), []byte(strings.Join(specs, "\n")+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("semantic-release cache: %w", err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		// Another run installed the same set first.
		if ready(dir) {
			return dir, nil
		}
		return "", fmt.Errorf("semantic-release cache: %w", err)
	}
	return dir, nil
}

// sweepStaleInstalls removes the staging directories in base last modified
// before cutoff. Failures are ignored: the sweep is housekeeping only.
func sweepStaleInstalls(base string, cutoff time.Time) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), installPrefix) {
			continue
		}
		if info, err := entry.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(base, entry.Name()))
		}
	}
}

func ready(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, readyMarker))
	return err == nil
}

// checkPins fails when the repository's semantic-release configuration needs
// a package that is neither bundled with semantic-release, pinned by default
// nor listed in release.semantic_release.extra_plugins, or names a shareable
// config that could not be loaded.
func checkPins(packages, unresolved, specs []string) error {
	allowed := map[string]bool{}
	for _, name := range bundledPackages {
		allowed[name] = true
	}
	for _, spec := range specs {
		name, _ := splitSpec(spec)
		allowed[name] = true
	}
	var missing []string
	for _, p := range packages {
		if !allowed[p] {
			missing = append(missing, p)
		}
	}
	var problems []string
	if len(missing) > 0 {
		problems = append(problems, fmt.Sprintf(
			"the semantic-release configuration needs %s, not installed by NERV; pin each one in .nerv/nerv.yaml under release.semantic_release.extra_plugins as \"<name>@<exact version>\"",
			strings.Join(missing, ", ")))
	}
	if len(unresolved) > 0 {
		problems = append(problems, fmt.Sprintf("the shareable config(s) %s named in extends could not be loaded", strings.Join(unresolved, ", ")))
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
