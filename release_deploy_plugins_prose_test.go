package nerv

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// removedCoreCommands are the release commands that moved out of core into
// the release and deploy extension plugins.
var removedCoreCommands = []string{"/nerv:ship", "/nerv:version", "/nerv:release-setup", "/nerv:pipeline"}

// Core nerv ships no release command and never names one: releasing,
// versioning and deploying belong to the release and deploy plugins.
func TestCorePlugin_HasNoReleaseCommands(t *testing.T) {
	for _, name := range []string{"release", "version", "release-setup", "pipeline"} {
		if _, err := fs.Stat(PluginFS(), "commands/"+name+".md"); err == nil {
			t.Errorf("plugin/commands/%s.md still exists; it moved to the release or deploy plugin", name)
		}
	}
	err := fs.WalkDir(PluginFS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(PluginFS(), path)
		if err != nil {
			return err
		}
		for _, cmd := range removedCoreCommands {
			if strings.Contains(string(data), cmd) {
				t.Errorf("plugin/%s still names %s", path, cmd)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// agentNamePattern reads an agent file's frontmatter name.
var agentNamePattern = regexp.MustCompile(`(?m)^name: (\S+)$`)

// Each plugin ships its own agents, commands and contract skill.
func TestReleaseDeployPlugins_HaveAgentsAndCommands(t *testing.T) {
	want := map[string]struct{ agents, commands []string }{
		"release": {agents: []string{"versioner", "builder"}, commands: []string{"version", "setup", "build"}},
		"deploy":  {agents: []string{"deployer"}, commands: []string{"run", "setup"}},
	}
	for plugin, w := range want {
		for _, agent := range w.agents {
			path := plugin + "/agents/" + agent + ".md"
			data, err := fs.ReadFile(ExtensionsFS(), path)
			if err != nil {
				t.Errorf("plugins/%s is missing: %v", path, err)
				continue
			}
			m := agentNamePattern.FindSubmatch(data)
			if m == nil || string(m[1]) != agent {
				t.Errorf("plugins/%s frontmatter name = %q, want %q", path, m, agent)
			}
		}
		for _, cmd := range w.commands {
			path := plugin + "/commands/" + cmd + ".md"
			data, err := fs.ReadFile(ExtensionsFS(), path)
			if err != nil {
				t.Errorf("plugins/%s is missing: %v", path, err)
				continue
			}
			if !strings.HasPrefix(string(data), "---\ndescription: ") {
				t.Errorf("plugins/%s has no description frontmatter", path)
			}
		}
		skill := plugin + "/skills/" + plugin + "/SKILL.md"
		if _, err := fs.Stat(ExtensionsFS(), skill); err != nil {
			t.Errorf("plugins/%s is missing: %v", skill, err)
		}
	}
}

// The setup commands tell the user how to import and where to install.
func TestReleaseDeployPlugins_SetupNamesImportAndProviders(t *testing.T) {
	read := func(path string) string {
		data, err := fs.ReadFile(ExtensionsFS(), path)
		if err != nil {
			t.Fatalf("reading plugins/%s: %v", path, err)
		}
		return string(data)
	}
	assertContainsAll(t, "release/commands/setup.md", read("release/commands/setup.md"),
		"nerv release setup --import", "build.mode", "compose", "script", "nerv version next")
	assertContainsAll(t, "deploy/commands/setup.md", read("deploy/commands/setup.md"),
		"deploy-github", "deploy-ssh-compose", "nerv install --with")
	assertContainsAll(t, "deploy/commands/run.md", read("deploy/commands/run.md"),
		"nerv deploy --artifact", "--dry-run", "--yes")
	assertContainsAll(t, "release/commands/build.md", read("release/commands/build.md"),
		"nerv release --dry-run", "--expect", ".nerv/artifacts/")
}

// semanticReleaseRun matches a command line that runs semantic-release.
var semanticReleaseRun = regexp.MustCompile(`(?im)(^\s*(\$\s*)?|\b(npx|npm\s+exec|pnpm\s+(dlx|exec)|yarn(\s+dlx)?|bunx)\s+)semantic-release(\s|$)`)

// The release and deploy plugins never run semantic-release: the version engine
// is native, and an existing semantic-release config is only imported.
func TestReleaseDeployPlugins_NeverRunSemanticRelease(t *testing.T) {
	for _, root := range []string{"release", "deploy", "deploy-github", "deploy-ssh-compose"} {
		err := fs.WalkDir(ExtensionsFS(), root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
				return err
			}
			data, err := fs.ReadFile(ExtensionsFS(), path)
			if err != nil {
				return err
			}
			if loc := semanticReleaseRun.FindIndex(data); loc != nil {
				t.Errorf("plugins/%s runs semantic-release: %q", path, strings.TrimSpace(string(data[loc[0]:loc[1]])))
			}
			return nil
		})
		if err != nil {
			t.Errorf("walking plugins/%s: %v", root, err)
		}
	}
}
