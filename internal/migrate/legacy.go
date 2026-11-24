package migrate

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

// alphaSection is the 5.0.0-alpha.1 name of the release: section.
const alphaSection = "ship"

// refuseLegacySection refuses section, an older nerv's (version) shape
// of the release and deploy configuration that nerv migrate no longer
// converts: those engines left nerv for nerv-release. The message names
// the file and the section to move by hand.
func refuseLegacySection(where, section, version string) error {
	return refuse("%s: %s: is a nerv %s section, which this nerv no longer converts; rewrite it by hand as nerv-release's release: and deploy: sections (see docs/migration.md), or run nerv migrate with nerv 6.6 first and this one after", where, section, version)
}

// bashTarget matches a provider: bash entry of a deploy target, in block
// or flow style, quoted or not.
var bashTarget = regexp.MustCompile(`(?m)provider:[ \t]*["']?bash["']?([ \t,}]|$)`)

// refuseScriptTargets refuses a deploy target named bash in doc's key
// block (deploy, or defaults for the user configuration's
// defaults.deploy): the deploy script target of nerv 5.x, script since.
func refuseScriptTargets(doc *config.Document, key, where string) error {
	if block, found := doc.Block(key); found && bashTarget.MatchString(block) {
		return refuse("%s: %s: a deploy target provider: bash is nerv 5.x's name, which this nerv no longer converts; rename it provider: script by hand", where, key)
	}
	return nil
}

// projectProvidersDir is where a repository keeps its own providers.
var projectProvidersDir = paths.ProjectDirName + "/providers"

// legacyDeployExtension matches the extension: deploy line of a nerv 5.x
// project deploy provider's provider.yaml.
var legacyDeployExtension = regexp.MustCompile(`(?m)^extension:[ \t]*["']?deploy["']?([ \t]*(#.*)?)$`)

// refuseProjectDeployProviders refuses a nerv 5.x project deploy
// provider of the repository at root (.ai-workflow/providers/deploy-<name>,
// extension: deploy): it is release-<name> of extension: release since.
func refuseProjectDeployProviders(root string) error {
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
		return refuse("%s/%s: a nerv 5.x project deploy provider, which this nerv no longer converts; move it to %s/release-%s and set extension: release in its provider.yaml by hand", projectProvidersDir, e.Name(), projectProvidersDir, name)
	}
	return nil
}
