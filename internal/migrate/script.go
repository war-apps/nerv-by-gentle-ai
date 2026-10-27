package migrate

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
)

// The deploy script target was named bash while it belonged to the deploy
// extension (deploy-bash); in the release extension, whose bash is a build
// provider, it is script (release-script).
const (
	removedScriptTarget = "bash"
	scriptTarget        = "script"
)

// bashTarget matches a provider: bash entry of a deploy target, in block
// or flow style, quoted or not.
var bashTarget = regexp.MustCompile(`(?m)(provider:[ \t]*["']?)bash(["']?)([ \t,}]|$)`)

// renameScriptTargets rewrites, in doc's key block (deploy, or defaults
// for the user configuration's defaults.deploy), every target provider
// bash to script, comments and layout kept, and reports whether it changed
// anything. Only deploy targets have a provider: key.
func renameScriptTargets(p *Plan, doc *config.Document, key, where string) bool {
	block, found := doc.Block(key)
	if !found || !bashTarget.MatchString(block) {
		return false
	}
	doc.SetBlock(key, bashTarget.ReplaceAllString(block, "${1}"+scriptTarget+"${2}${3}"))
	p.note("%s: deploy target provider %s renamed %s (the deploy script target of the release extension, release-%s)", where, removedScriptTarget, scriptTarget, scriptTarget)
	return true
}

// legacyDeployExtension matches the extension: deploy line of a project
// deploy provider's provider.yaml.
var legacyDeployExtension = regexp.MustCompile(`(?m)^extension:[ \t]*["']?deploy["']?([ \t]*(#.*)?)$`)

// moveProjectDeployProviders moves each project deploy provider of the
// repository at root, .ai-workflow/providers/deploy-<name>, to
// release-<name>, its provider.yaml's extension: deploy rewritten release:
// a deploy provider belongs to the release extension now. A destination
// that already exists, or a name the release extension's build providers
// use, is refused.
func moveProjectDeployProviders(p *Plan, root string) error {
	dir := filepath.Join(root, filepath.FromSlash(extension.ProjectProvidersDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		name, ok := strings.CutPrefix(e.Name(), "deploy-")
		if !ok || !e.IsDir() {
			continue
		}
		from, to := filepath.Join(dir, e.Name()), filepath.Join(dir, "release-"+name)
		rel := extension.ProjectProvidersDir + "/" + e.Name()
		if _, err := os.Lstat(to); err == nil {
			return refuse("%s cannot move: %s/release-%s already exists; merge them by hand", rel, extension.ProjectProvidersDir, name)
		}
		if name == removedScriptTarget {
			return refuse("%s: a project deploy provider named %s would be release-%s, the release extension's build provider; rename it (its folder and name:) and run nerv migrate again", rel, name, name)
		}
		err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sub, _ := filepath.Rel(from, path)
			dst := filepath.Join(to, sub)
			if sub == extension.ProviderFile && legacyDeployExtension.Match(data) {
				p.Changes = append(p.Changes,
					Change{Kind: Write, Path: dst, New: legacyDeployExtension.ReplaceAll(data, []byte("extension: release$1")), Why: "the deploy provider, now of the release extension"},
					Change{Kind: Remove, Path: path, Old: data, StopAt: dir, Why: "moved to " + extension.ProjectProvidersDir + "/release-" + name})
				return nil
			}
			p.Changes = append(p.Changes, Change{Kind: Rename, Path: path, To: dst, Old: data, StopAt: dir})
			return nil
		})
		if err != nil {
			return err
		}
		p.note("%s moved to release-%s: the deploy extension merged into release", rel, name)
	}
	return nil
}
