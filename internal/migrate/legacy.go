package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"gopkg.in/yaml.v3"
)

// alphaSection is the 5.0.0-alpha.1 name of the release: section.
const alphaSection = "ship"

// installNervRelease is the advice of a refusal of an older nerv's
// release and deploy configuration: with nerv-release installed, nerv
// migrate hands it to nerv-release migrate, which converts it (S4).
const installNervRelease = "install nerv-release (nerv install --tools nerv-release) and run nerv migrate again, which converts it"

// refuseLegacySection refuses section, an older nerv's (version) shape
// of the release and deploy configuration, when nerv-release is not
// installed. The message names the file and the section, how it was told
// apart (tell, "" when its name is enough), and the command that installs
// nerv-release.
func refuseLegacySection(where, section, version, tell string) error {
	if tell != "" {
		tell = " (it has " + tell + ")"
	}
	return refuse("%s: %s: is a nerv %s section%s, which nerv-release converts; %s, or rewrite it by hand as nerv-release's release: and deploy: sections (see docs/migration.md)", where, section, version, tell, installNervRelease)
}

// legacyHandOff collects the older release and deploy configuration a
// plan leaves for nerv-release migrate (S5). With no target (nerv-release
// not installed) each one found is refused instead.
type legacyHandOff struct {
	// target is the release.yaml the sections go to ("" refuses them).
	target string
	found  []string
	// left are the older shapes target itself still holds, moved there
	// by an earlier run whose nerv-release migrate failed or refused.
	left []string
}

// take records what (such as ".nerv/nerv.yaml pipeline:"), or returns
// refusal when nerv-release is not installed.
func (h *legacyHandOff) take(what string, refusal error) error {
	if h.target == "" {
		return refusal
	}
	h.found = append(h.found, what)
	return nil
}

// legacyReleaseSections are the top-level sections only an older nerv
// had (4.2.0's pipeline:, 5.0.0-alpha.1's ship:): with nerv-release they
// move, under the same names, to release.yaml as release: and deploy: do.
var legacyReleaseSections = []string{"pipeline", alphaSection}

// legacyReleaseKeys are the keys only a 4.2.0 release: section had, at
// its top, and legacyProjectKeys those of its projects (the presets):
// they tell it apart from today's release: section.
var (
	legacyReleaseKeys = []string{"semantic_release"}
	legacyProjectKeys = []string{"presets", "dotnet", "node", "docker", "nuget", "npm"}
)

// takeLegacyRelease hands off (or refuses) a 4.2.0 release: section of
// doc (the file at where), told apart by its 4.2.0 keys, naming them.
func takeLegacyRelease(h *legacyHandOff, doc *config.Document, where string) error {
	found, err := legacyReleaseKeysOf(doc)
	if err != nil {
		return refuse("%s is not valid YAML: %v", where, err)
	}
	if len(found) == 0 {
		return nil
	}
	return h.take(where+" release: (4.2.0)", refuseLegacySection(where, "release", "4.2.0", strings.Join(found, ", ")))
}

// legacyReleaseKeysOf lists the 4.2.0 keys of doc's release: section
// (legacyReleaseKeys at its top, legacyProjectKeys in its projects), each
// with its path; none for today's section.
func legacyReleaseKeysOf(doc *config.Document) ([]string, error) {
	root, err := parseYAML(doc.String())
	if err != nil {
		return nil, err
	}
	release := mapping(root, "release")
	var found []string
	for _, k := range keys(release) {
		if slices.Contains(legacyReleaseKeys, k) {
			found = append(found, "release."+k)
		}
	}
	if projects := value(release, "projects"); projects != nil && projects.Kind == yaml.SequenceNode {
		for i, pr := range projects.Content {
			for _, k := range keys(pr) {
				if slices.Contains(legacyProjectKeys, k) {
					found = append(found, fmt.Sprintf("release.projects[%d].%s", i, k))
				}
			}
		}
	}
	return found, nil
}

// movesAsText reports whether doc's section is an older nerv's that moves
// to release.yaml as its own text (comments and layout kept), for
// nerv-release migrate to convert: 4.2.0's pipeline:, 5.0.0-alpha.1's
// ship:, a 4.2.0 release:, and a deploy: with a provider: bash target
// and no personal key (those stay in the project file, reported).
func movesAsText(doc *config.Document, section string) bool {
	switch section {
	case "release":
		found, err := legacyReleaseKeysOf(doc)
		return err == nil && len(found) > 0
	case "deploy":
		block, _ := doc.Block(section)
		return bashTarget.MatchString(block) && len(presentReleasePersonal(doc)) == 0
	}
	return slices.Contains(legacyReleaseSections, section)
}

// bashTarget matches a provider: bash entry of a deploy target, in block
// or flow style, quoted or not.
var bashTarget = regexp.MustCompile(`(?m)provider:[ \t]*["']?bash["']?([ \t,}]|$)`)

// takeScriptTargets hands off (or refuses) a deploy target named bash in
// doc's key block (deploy, or defaults for the user configuration's
// defaults.deploy): the deploy script target of nerv 5.x, script since.
func takeScriptTargets(h *legacyHandOff, doc *config.Document, key, where string) error {
	if block, found := doc.Block(key); found && bashTarget.MatchString(block) {
		return h.take(where+" "+key+": (provider: bash)", refuse("%s: %s: a deploy target provider: bash is nerv 5.x's name, which nerv-release converts; rename it provider: script by hand, or %s", where, key, installNervRelease))
	}
	return nil
}

// projectProvidersDir is where a repository keeps its own providers.
var projectProvidersDir = paths.ProjectDirName + "/providers"

// legacyDeployExtension matches the extension: deploy line of a nerv 5.x
// project deploy provider's provider.yaml.
var legacyDeployExtension = regexp.MustCompile(`(?m)^extension:[ \t]*["']?deploy["']?([ \t]*(#.*)?)$`)

// takeProjectDeployProviders hands off (or refuses) a nerv 5.x project
// deploy provider of the repository at root
// (.ai-workflow/providers/deploy-<name>, extension: deploy): it is
// release-<name> of extension: release since, which nerv-release migrate
// renames; nerv migrate leaves the folder as it is.
func takeProjectDeployProviders(h *legacyHandOff, root string) error {
	dir := filepath.Join(root, filepath.FromSlash(projectProvidersDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		name, ok := strings.CutPrefix(e.Name(), "deploy-")
		if !ok || !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name(), "provider.yaml"))
		if err != nil || !legacyDeployExtension.Match(data) {
			continue
		}
		err = refuse("%s/%s: a nerv 5.x project deploy provider, which nerv-release converts; move it to %s/release-%s and set extension: release in its provider.yaml by hand, or %s", projectProvidersDir, e.Name(), projectProvidersDir, name, installNervRelease)
		if err := h.take(projectProvidersDir+"/"+e.Name(), err); err != nil {
			return err
		}
	}
	return nil
}

// takeReleaseYAML records the older nerv's shapes the release.yaml h
// hands off to (h.target) still holds: an earlier nerv migrate moved them
// there, and its nerv-release migrate failed or refused, so they are
// handed to nerv-release migrate again even when nerv has nothing to
// write. The shapes are those that move as text (movesAsText): 4.2.0's
// pipeline:, 5.0.0-alpha.1's ship:, a 4.2.0 release:, and a provider:
// bash target under deploy: or defaults: (the global file's seeds). A
// no-op without nerv-release, or when the file does not exist yet.
func (h *legacyHandOff) takeReleaseYAML() error {
	if h.target == "" {
		return nil
	}
	data, exists, err := readOptional(h.target)
	if err != nil || !exists {
		return err
	}
	doc := config.Parse(string(data))
	for _, section := range legacyReleaseSections {
		if doc.KeyExists(section) {
			h.left = append(h.left, section+":")
		}
	}
	if movesAsText(doc, "release") {
		h.left = append(h.left, "release: (4.2.0)")
	}
	for _, key := range []string{"deploy", "defaults"} {
		if block, found := doc.Block(key); found && bashTarget.MatchString(block) {
			h.left = append(h.left, key+": (provider: bash)")
		}
	}
	return nil
}

// finish records the hand-off on p, setting flag (p.LegacyReleaseRepo or
// p.LegacyReleaseUser), when anything was found, or is still left in
// the target release.yaml.
func (h *legacyHandOff) finish(p *Plan, flag *bool) {
	if len(h.left) > 0 {
		*flag = true
		p.note("%s still holds an older nerv's release and deploy configuration (%s): nerv-release migrate converts it, and nerv migrate runs it", h.target, strings.Join(h.left, ", "))
	}
	if len(h.found) == 0 {
		return
	}
	*flag = true
	p.note("an older nerv's release and deploy configuration (%s) is left for nerv-release migrate to convert: its sections move as they are to %s, and nerv migrate runs nerv-release migrate once these steps are written", strings.Join(h.found, ", "), h.target)
}
