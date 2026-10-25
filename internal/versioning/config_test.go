package versioning

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestParseConfigDefaults(t *testing.T) {
	c, err := ParseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.NoteKeywords, DefaultNoteKeywords()) || c.TagFormat != "v${version}" || c.Changelog.File != "CHANGELOG.md" {
		t.Fatalf("defaults: %+v", c)
	}
	if len(c.Branches) != 4 || c.Branches[1].Name != "develop" || c.Branches[1].Prerelease.ID() != "alpha" || c.Branches[2].Channel != "beta" || c.Branches[3].Prerelease.ID() != "beta" {
		t.Fatalf("default branches: %+v", c.Branches)
	}
}

func TestParseConfigReadsEverySetting(t *testing.T) {
	c, err := ParseConfig([]byte(`
branches:
  - main
  - name: beta
    prerelease: true
  - name: release/*
    prerelease: rc
    channel: next
release_rules:
  - type: docs
    scope: readme
    release: patch
  - type: feat
    scope: wip
    release: false
  - breaking: true
    release: minor
note_keywords: [BREAKING CHANGE]
tag_format: rel-${version}
repository_url: https://github.com/acme/widget
changelog:
  file: HISTORY.md
  title: "# History"
projects:
  - name: api
    path: src/api
    shared_paths: [build/*.props]
    depends_on:
      - project: lib
        bump: minor
  - name: lib
    path: src\lib
    scopes: [lib, core]
    tag_format: lib@${version}
    changelog: docs/LIB.md
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Branches[0].Name != "main" || !c.Branches[1].Prerelease.UsesName() || c.Branches[2].Prerelease.ID() != "rc" || c.Branches[2].Channel != "next" {
		t.Fatalf("branches: %+v", c.Branches)
	}
	if c.ReleaseRules[1].Release != ReleaseNone || !c.ReleaseRules[2].Breaking || c.ReleaseRules[0].Scope != "readme" {
		t.Fatalf("rules: %+v", c.ReleaseRules)
	}
	api, lib := c.Projects[0], c.Projects[1]
	if api.TagFormat != "api-rel-${version}" || api.Changelog != "src/api/HISTORY.md" || !slices.Equal(api.Scopes, []string{"api"}) || api.DependsOn[0].Bump != "minor" {
		t.Fatalf("api: %+v", api)
	}
	if lib.Path != "src/lib" || lib.TagFormat != "lib@${version}" || lib.Changelog != "docs/LIB.md" || c.Changelog.Title != "# History" {
		t.Fatalf("lib: %+v", lib)
	}
}

func TestParseConfigSingleProjectKeepsTheTagFormat(t *testing.T) {
	c, err := ParseConfig([]byte("projects:\n  - name: app\n    path: .\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Projects[0].TagFormat != "v${version}" || c.Projects[0].Changelog != "CHANGELOG.md" {
		t.Fatalf("project: %+v", c.Projects[0])
	}
}

func TestParseConfigRefusesUnknownKeys(t *testing.T) {
	for _, doc := range []string{
		"branchez: [main]\n",
		"projects:\n  - name: a\n    pth: a\n",
		"release_rules:\n  - type: feat\n    subject: x\n    release: patch\n",
		"branches:\n  - name: main\n    range: 1.x\n",
	} {
		_, err := ParseConfig([]byte(doc))
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("%q: err = %v, want an unknown field", doc, err)
		}
	}
}

func TestParseConfigValidates(t *testing.T) {
	cases := map[string]string{
		"tag_format: v1\n":                                   "exactly once",
		"tag_format: ${version}-${channel}\n":                "placeholder",
		"release_rules:\n  - type: feat\n":                   "release is required",
		"release_rules:\n  - type: feat\n    release: big\n": "is not major",
		"projects:\n  - name: A\n    path: a\n":              "must match",
		"projects:\n  - name: a\n    path: ../x\n":           "escapes",
		"projects:\n  - name: a\n    path: a\n  - name: b\n    path: b\n    tag_format: a-v${version}\n":                                  "also the tag format",
		"projects:\n  - name: a\n    path: a\n    depends_on: [{project: z}]\n":                                                           "unknown project",
		"projects:\n  - name: a\n    path: a\n    depends_on: [{project: b}]\n  - name: b\n    path: b\n    depends_on: [{project: a}]\n": "cycle",
		"projects:\n  - name: a\n    path: a\n    depends_on: [{project: a}]\n":                                                           "itself",
	}
	for doc, want := range cases {
		_, err := ParseConfig([]byte(doc))
		var ce *ConfigError
		if !errors.As(err, &ce) || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want a ConfigError with %q", doc, err, want)
		}
	}
}
