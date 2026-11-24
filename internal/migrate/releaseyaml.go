package migrate

import (
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

// releaseSections are the sections nerv-release reads from its own
// release.yaml, under the same names (unlike task.yaml, the keys keep
// their prefix).
var releaseSections = []string{"release", "deploy"}

// releaseDefaultsSections are the user config's seeds nerv-release reads
// from the global release.yaml's defaults:, under the same names.
var releaseDefaultsSections = []string{"defaults.release", "defaults.deploy"}

// releasePersonalKeys are the only personal (scope: user) release keys:
// the ssh-compose deploy provider's machine-local settings. nerv-release
// reads them from the global release.yaml only; they never move from a
// repository into any release.yaml.
var releasePersonalKeys = slices.DeleteFunc(slices.Clone(workflowconfig.PersonalProviderKeys),
	func(k string) bool { return !strings.HasPrefix(k, "deploy.") })

func isReleasePersonal(path string) bool { return slices.Contains(releasePersonalKeys, path) }

// moveRepoReleaseYAML moves doc's release: and deploy: sections into the
// repository's own release.yaml (o.ReleaseYAMLRepo), key by key, each keeping its "release."/"deploy."
// prefix. A personal key (releasePersonalKeys) is never moved: it stays
// in doc and is reported. A key already set in release.yaml is kept in
// doc and reported as a conflict. A no-op when o.ReleaseYAMLRepo is "".
func moveRepoReleaseYAML(p *Plan, o Options, doc *config.Document) error {
	if o.ReleaseYAMLRepo == "" {
		return nil
	}
	for _, path := range presentReleasePersonal(doc) {
		p.warn("%s: %s: a personal setting in the project file, kept: nerv-release reads it from your global release.yaml only; set it there and remove it here", paths.ProjectConfigFile, path)
	}
	existing, exists, err := readOptional(o.ReleaseYAMLRepo)
	if err != nil {
		return err
	}
	releaseDoc := config.Parse(string(existing))
	var moved []string
	for _, section := range releaseSections {
		skip := func(leaf string) bool { return isReleasePersonal(section + "." + leaf) }
		migrated, err := migrateSection(p, doc, section, releaseDoc, section, paths.ProjectConfigFile, "the repository release.yaml", skip)
		if err != nil {
			return err
		}
		removeMigrated(doc, section, migrated)
		moved = append(moved, migrated...)
	}
	if len(moved) == 0 {
		return nil
	}
	text := newFileText(releaseDoc, exists)
	if err := verifyMigratedYAML(o.ReleaseYAMLRepo, text); err != nil {
		return err
	}
	c := Change{Kind: Write, Path: o.ReleaseYAMLRepo, New: []byte(text), Why: "nerv-release's repository configuration, moved from " + paths.ProjectConfigFile}
	if exists {
		c.Old = existing
	}
	p.Changes = append(p.Changes, c)
	p.note("%s: %s moved to %s", paths.ProjectConfigFile, strings.Join(moved, ", "), o.ReleaseYAMLRepo)
	return nil
}

// withoutReleasePersonal is doc's text without its personal release keys
// (releasePersonalKeys), for checkLoads once nerv-release owns them: they
// are reported by moveRepoReleaseYAML instead of refused.
func withoutReleasePersonal(doc *config.Document) string {
	clone := config.Parse(doc.String())
	removeMigrated(clone, "deploy", presentReleasePersonal(clone))
	return clone.String()
}

// presentReleasePersonal lists the personal release keys
// (releasePersonalKeys) doc sets.
func presentReleasePersonal(doc *config.Document) []string {
	root, err := parseYAML(doc.String())
	if err != nil {
		return nil
	}
	var present []string
	for _, path := range releasePersonalKeys {
		if nodeAt(root, path) != nil {
			present = append(present, path)
		}
	}
	return present
}

// moveUserReleaseYAML moves the user config's personal release keys
// (releasePersonalKeys, under release:/deploy:) and its
// defaults.release:/defaults.deploy: seeds into the global release.yaml
// (o.ReleaseYAMLHome), every key at the same path (prefix kept). A
// non-personal key left under the user's release:/deploy: (one
// moveUserDefaults could not move, already reported) stays. A key already
// set in release.yaml is kept in the user config and reported as a
// conflict. A no-op when o.ReleaseYAMLHome is "".
func moveUserReleaseYAML(p *Plan, o Options, u *userState) error {
	if o.ReleaseYAMLHome == "" {
		return nil
	}
	existing, exists, err := readOptional(o.ReleaseYAMLHome)
	if err != nil {
		return err
	}
	releaseDoc := config.Parse(string(existing))
	const where, target = "user config", "the global release.yaml"
	var personal, defaults []string
	for _, section := range releaseSections {
		skip := func(leaf string) bool { return !isReleasePersonal(section + "." + leaf) }
		migrated, err := migrateSection(p, u.doc, section, releaseDoc, section, where, target, skip)
		if err != nil {
			return err
		}
		removeMigrated(u.doc, section, migrated)
		personal = append(personal, migrated...)
	}
	for _, section := range releaseDefaultsSections {
		migrated, err := migrateSection(p, u.doc, section, releaseDoc, section, where, target, nil)
		if err != nil {
			return err
		}
		removeMigrated(u.doc, section, migrated)
		defaults = append(defaults, migrated...)
	}
	if len(personal) == 0 && len(defaults) == 0 {
		return nil
	}
	if len(defaults) > 0 {
		if root, err := parseYAML(u.doc.String()); err == nil && isEmptyNode(mapping(root, "defaults")) {
			u.doc.Remove("defaults")
		}
	}
	text := newFileText(releaseDoc, exists)
	if err := verifyMigratedYAML(o.ReleaseYAMLHome, text); err != nil {
		return err
	}
	c := Change{Kind: Write, Path: o.ReleaseYAMLHome, New: []byte(text), User: true, Why: "nerv-release's global configuration, moved from your user configuration"}
	if exists {
		c.Old = existing
	}
	u.destinations = append(u.destinations, c)
	moved := append(slices.Clone(personal), defaults...)
	p.note("user config: %s moved to %s", strings.Join(moved, ", "), o.ReleaseYAMLHome)
	return nil
}

// newFileText is doc's text, without the leading blank line SetScalar
// leaves when it builds a new file from nothing (exists false).
func newFileText(doc *config.Document, exists bool) string {
	if exists {
		return doc.String()
	}
	return strings.TrimLeft(doc.String(), "\r\n")
}
