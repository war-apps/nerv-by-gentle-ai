package versioning

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeFiles writes files (name -> content) into a new temporary directory.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "import", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func warningsAt(ws []Warning) []string {
	var out []string
	for _, w := range ws {
		out = append(out, w.Where)
	}
	return out
}

func TestImportDimPackageJSON(t *testing.T) {
	for _, c := range []struct{ fixture, changelog, repo string }{
		{"comprobantes-package.json", "src/Api/CHANGELOG.md", "https://github.com/dirm-s-a/dim.comprobantes.api.git"},
		{"factura-package.json", "src/Host/CHANGELOG.md", "https://github.com/dirm-s-a/dim-factura-electronica-service.git"},
	} {
		t.Run(c.fixture, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"package.json": fixture(t, c.fixture)})
			res, err := Import(dir)
			if err != nil {
				t.Fatal(err)
			}
			if res.Source != "package.json" {
				t.Fatalf("source = %q", res.Source)
			}
			cfg := res.Config
			want := []Branch{
				{Name: "main"},
				{Name: "release/rc", Prerelease: PrereleaseID("rc"), Channel: "beta"},
				{Name: "develop", Prerelease: PrereleaseID("alpha"), Channel: "alpha"},
				{Name: "hotfix/*", Prerelease: PrereleaseFromName(), Channel: "beta"},
			}
			if !slices.Equal(cfg.Branches, want) {
				t.Fatalf("branches = %+v, want %+v", cfg.Branches, want)
			}
			if !slices.Equal(cfg.NoteKeywords, DefaultNoteKeywords()) || cfg.Changelog.File != c.changelog || cfg.RepositoryURL != c.repo || cfg.TagFormat != "v${version}" || len(cfg.ReleaseRules) != 0 {
				t.Fatalf("config = %+v", cfg)
			}
			at := warningsAt(res.Warnings)
			for _, w := range []string{"@semantic-release/npm", "@semantic-release/exec", "@semantic-release/git", "@semantic-release/github", "branches[3]"} {
				if !slices.Contains(at, w) {
					t.Errorf("no warning at %s; warnings: %v", w, res.Warnings)
				}
			}
			for _, w := range res.Warnings {
				if w.Where == "@semantic-release/exec" && !strings.Contains(w.Message, "bump-csproj.sh") {
					t.Errorf("exec warning does not name its commands: %q", w.Message)
				}
			}
			if err := cfg.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestImportReleasercYAML(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"package.json": `{"name": "x"}`,
		".releaserc.yaml": `
extends: "@acme/release-config"
branches: [main, {name: next, channel: next}, {name: "1.x", range: "1.x"}]
tagFormat: app-v${version}
plugins:
  - - "@semantic-release/commit-analyzer"
    - preset: angular
      presetConfig: {types: []}
      releaseRules:
        - {type: docs, scope: README, release: patch}
        - {type: refactor, release: false}
        - {subject: "*skip*", release: false}
      parserOpts: {noteKeywords: [BREAKING], headerPattern: "x"}
  - "@semantic-release/release-notes-generator"
  - ["@semantic-release/changelog", {changelogFile: HISTORY.md, changelogTitle: "# History"}]
  - "semantic-release-slack-bot"
`,
	})
	res, err := Import(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := res.Config
	if res.Source != ".releaserc.yaml" || cfg.TagFormat != "app-v${version}" || cfg.Changelog.File != "HISTORY.md" || cfg.Changelog.Title != "# History" {
		t.Fatalf("config = %+v (source %q)", cfg, res.Source)
	}
	if len(cfg.Branches) != 2 || cfg.Branches[1].Channel != "next" {
		t.Fatalf("branches = %+v", cfg.Branches)
	}
	wantRules := []ReleaseRule{{Type: "docs", Scope: "README", Release: ReleasePatch}, {Type: "refactor", Release: ReleaseNone}}
	if !slices.Equal(cfg.ReleaseRules, wantRules) || !slices.Equal(cfg.NoteKeywords, []string{"BREAKING"}) {
		t.Fatalf("rules = %+v keywords = %v", cfg.ReleaseRules, cfg.NoteKeywords)
	}
	at := warningsAt(res.Warnings)
	for _, w := range []string{"extends", "branches[2]", "commit-analyzer.preset", "commit-analyzer.presetConfig", "commit-analyzer.releaseRules[2]", "commit-analyzer.parserOpts.headerPattern", "release-notes-generator.preset", "semantic-release-slack-bot"} {
		if !slices.Contains(at, w) {
			t.Errorf("no warning at %s; warnings: %v", w, res.Warnings)
		}
	}
}

func TestImportDefaultPluginsAndBranches(t *testing.T) {
	dir := writeFiles(t, map[string]string{".releaserc": `{"tagFormat": "${version}"}`})
	res, err := Import(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, b := range res.Config.Branches {
		names = append(names, b.Name)
	}
	if !slices.Equal(names, []string{"master", "main", "next", "next-major", "beta", "alpha"}) || !res.Config.Branches[4].Prerelease.UsesName() {
		t.Fatalf("branches = %+v", res.Config.Branches)
	}
	at := warningsAt(res.Warnings)
	for _, w := range []string{"branches", "@semantic-release/npm", "commit-analyzer.preset"} {
		if !slices.Contains(at, w) {
			t.Errorf("no warning at %s; warnings: %v", w, res.Warnings)
		}
	}
}

func TestImportRefusesJavaScript(t *testing.T) {
	for _, name := range []string{".releaserc.js", ".releaserc.cjs", ".releaserc.mjs", "release.config.js", "release.config.cjs", "release.config.mjs"} {
		dir := writeFiles(t, map[string]string{name: "module.exports = {}"})
		_, err := Import(dir)
		var manual *ManualMigrationError
		if !errors.As(err, &manual) || manual.File != name || !strings.Contains(err.Error(), "by hand") {
			t.Errorf("%s: err = %v, want a ManualMigrationError", name, err)
		}
	}
}

func TestImportWithoutConfig(t *testing.T) {
	dir := writeFiles(t, map[string]string{"package.json": `{"name": "x"}`})
	if _, err := Import(dir); !errors.Is(err, ErrNoSemanticReleaseConfig) {
		t.Fatalf("err = %v, want ErrNoSemanticReleaseConfig", err)
	}
}
