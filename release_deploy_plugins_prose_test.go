package nerv

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// removedCoreCommands are the release commands that moved out of core into
// the release extension plugin.
var removedCoreCommands = []string{"/nerv:ship", "/nerv:version", "/nerv:release-setup", "/nerv:pipeline"}

// Core nerv ships no release command and never names one: releasing,
// versioning and deploying belong to the release plugin.
func TestCorePlugin_HasNoReleaseCommands(t *testing.T) {
	for _, name := range []string{"release", "version", "release-setup", "pipeline"} {
		if _, err := fs.Stat(PluginFS(), "commands/"+name+".md"); err == nil {
			t.Errorf("plugin/commands/%s.md still exists; it moved to the release plugin", name)
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

// The release plugin ships its agents, commands and contract skills,
// deploy's among them; the deploy plugin is gone.
func TestReleaseDeployPlugins_HaveAgentsAndCommands(t *testing.T) {
	want := map[string]struct{ agents, commands, skills []string }{
		"release": {
			agents:   []string{"versioner", "builder", "deployer"},
			commands: []string{"version", "setup", "build", "deploy", "deploy-setup", "ship"},
			skills:   []string{"release", "deploy"},
		},
	}
	if _, err := fs.Stat(ExtensionsFS(), "deploy"); err == nil {
		t.Errorf("plugins/deploy still exists; it merged into plugins/release")
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
		for _, s := range w.skills {
			skill := plugin + "/skills/" + s + "/SKILL.md"
			if _, err := fs.Stat(ExtensionsFS(), skill); err != nil {
				t.Errorf("plugins/%s is missing: %v", skill, err)
			}
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
		"nerv release setup --import", "build provider", "docker-compose", "bash", "nerv version next")
	assertContainsAll(t, "release/commands/deploy-setup.md", read("release/commands/deploy-setup.md"),
		"release-github", "release-ssh-compose", "release-script")
	assertContainsAll(t, "release/commands/deploy.md", read("release/commands/deploy.md"),
		"nerv deploy --artifact", "--dry-run", "--yes")
	assertContainsAll(t, "release/commands/build.md", read("release/commands/build.md"),
		"nerv release --dry-run", "--expect", ".nerv/artifacts/")
	assertContainsAll(t, "release/commands/ship.md", read("release/commands/ship.md"),
		"nerv release --dry-run", "--expect", "nerv deploy --artifact", "--dry-run", "--yes")
	// L1 of merge-deploy-into-release: version, build (lint and tests from
	// the configured build provider) and deploy, the deploy skipped when no
	// deploy provider is configured.
	assertContainsAll(t, "release/commands/ship.md", read("release/commands/ship.md"),
		"`deploy.host`", "`deploy.targets`", "deploy is skipped", "/release:deploy-setup",
		"`lint`", "`test`", "nerv adds no stage of its own", "the artifacts stay local")
}

// semanticReleaseRun matches a command line that runs semantic-release.
var semanticReleaseRun = regexp.MustCompile(`(?im)(^\s*(\$\s*)?|\b(npx|npm\s+exec|pnpm\s+(dlx|exec)|yarn(\s+dlx)?|bunx)\s+)semantic-release(\s|$)`)

// The release plugins never run semantic-release: the version engine
// is native, and an existing semantic-release config is only imported.
func TestReleaseDeployPlugins_NeverRunSemanticRelease(t *testing.T) {
	for _, root := range []string{"release", "release-conventional-commits", "release-docker-compose", "release-bash", "release-github", "release-ssh-compose", "release-script"} {
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
