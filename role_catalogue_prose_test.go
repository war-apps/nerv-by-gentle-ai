package nerv_test

import (
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/models"
)

// These guards pin the prose that copies role-catalogue data (the roles table
// in docs/configuration.md, the group legend in /nerv:configure and the
// wizard's group descriptions) to config.Roles() and the agents' frontmatter.
// They only read the prose; they never generate it.

// docsRoleRow is one parsed row of the roles table in docs/configuration.md.
type docsRoleRow struct {
	name, group, purpose, equivalent, defaults string
}

func stripTicks(s string) string { return strings.Trim(strings.TrimSpace(s), "`") }

// equivalentsCell normalises a "gentle-ai equivalent" cell such as
// "`jd-judge-a`, `review-readability`" to "jd-judge-a, review-readability".
func equivalentsCell(cell string) string {
	parts := strings.Split(cell, ",")
	for i, p := range parts {
		parts[i] = stripTicks(p)
	}
	return strings.Join(parts, ", ")
}

// parseDocsRolesTable returns the rows of the "Roles and their gentle-ai
// equivalents" table keyed by role.
func parseDocsRolesTable(t *testing.T) map[string]docsRoleRow {
	t.Helper()
	data, err := os.ReadFile("docs/configuration.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, found := strings.Cut(string(data), "### Roles and their gentle-ai equivalents")
	if !found {
		t.Fatal("docs/configuration.md has no 'Roles and their gentle-ai equivalents' section")
	}
	rows := map[string]docsRoleRow{}
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "### ") {
			break
		}
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), "|")
		if len(cells) != 6 {
			t.Fatalf("docs roles table row has %d cells, want 6: %q", len(cells), line)
		}
		role := stripTicks(cells[0])
		if _, dup := rows[role]; dup {
			t.Errorf("docs roles table lists role %q twice", role)
		}
		rows[role] = docsRoleRow{
			name:       strings.TrimSpace(cells[1]),
			group:      stripTicks(cells[2]),
			purpose:    strings.TrimSpace(cells[3]),
			equivalent: equivalentsCell(cells[4]),
			defaults:   strings.TrimSpace(cells[5]),
		}
	}
	return rows
}

func TestDocsRolesTable_MatchesCatalogue(t *testing.T) {
	cat := config.Roles()
	defaults, err := models.PluginDefaults(nerv.PluginFS())
	if err != nil {
		t.Fatal(err)
	}
	rows := parseDocsRolesTable(t)

	groupOf := map[string]string{}
	for _, group := range []string{"magi", "pilots", "audit-passes"} {
		for _, role := range cat.Groups[group] {
			groupOf[role] = group
		}
	}

	for _, role := range cat.AllRoles {
		row, ok := rows[role]
		if !ok {
			t.Errorf("docs roles table is missing role %q", role)
			continue
		}
		info := cat.Info[role]
		if row.name != info.DisplayName {
			t.Errorf("docs roles table: role %q name = %q, catalogue = %q", role, row.name, info.DisplayName)
		}
		if row.purpose != info.Purpose {
			t.Errorf("docs roles table: role %q purpose = %q, catalogue = %q", role, row.purpose, info.Purpose)
		}
		wantEquivalent := strings.Join(info.GentleAIEquivalents, ", ")
		if wantEquivalent == "" {
			wantEquivalent = "none"
		}
		if row.equivalent != wantEquivalent {
			t.Errorf("docs roles table: role %q gentle-ai equivalent = %q, catalogue = %q", role, row.equivalent, wantEquivalent)
		}
		if row.group != groupOf[role] {
			t.Errorf("docs roles table: role %q group = %q, catalogue = %q", role, row.group, groupOf[role])
		}
		wantDefaults := defaults[role].Model + " / " + defaults[role].Effort
		if row.defaults != wantDefaults {
			t.Errorf("docs roles table: role %q default model / effort = %q, agent frontmatter = %q", role, row.defaults, wantDefaults)
		}
	}
	for role := range rows {
		if _, ok := cat.Info[role]; !ok {
			t.Errorf("docs roles table lists %q, which is not a catalogue role", role)
		}
	}
}

var legendEntryRe = regexp.MustCompile("`(magi|pilots|audit-passes)` = (?:the [a-z ]+ )?\\(([^)]*)\\)")

func namesIn(list string) []string {
	var names []string
	for _, n := range strings.Split(list, ",") {
		names = append(names, strings.TrimSpace(n))
	}
	sort.Strings(names)
	return names
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestConfigureCommandLegend_MatchesCatalogueGroups(t *testing.T) {
	data, err := fs.ReadFile(nerv.PluginFS(), "commands/configure.md")
	if err != nil {
		t.Fatal(err)
	}
	// The legend wraps across lines; normalise whitespace before matching.
	text := strings.Join(strings.Fields(string(data)), " ")
	groups := config.Roles().Groups
	found := map[string]bool{}
	for _, m := range legendEntryRe.FindAllStringSubmatch(text, -1) {
		group := m[1]
		found[group] = true
		got, want := namesIn(m[2]), sortedCopy(groups[group])
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("commands/configure.md legend: group %q members = %v, catalogue = %v", group, got, want)
		}
	}
	for _, group := range []string{"magi", "pilots", "audit-passes"} {
		if !found[group] {
			t.Errorf("commands/configure.md has no legend entry for group %q", group)
		}
	}
	// `all` lists no members; its legend entry must carry the catalogue's
	// description instead.
	allDesc := config.Roles().GroupDescriptions["all"]
	if strings.TrimSpace(allDesc) == "" {
		t.Fatal(`GroupDescriptions["all"] is empty`)
	}
	if want := "`all` = " + allDesc; !strings.Contains(text, want) {
		t.Errorf("commands/configure.md legend lacks %q", want)
	}
}

func TestGroupDescriptions_NameTheGroupMembers(t *testing.T) {
	cat := config.Roles()
	parens := regexp.MustCompile(`\(([^)]*)\)`)
	for _, group := range []string{"magi", "pilots", "audit-passes"} {
		m := parens.FindStringSubmatch(cat.GroupDescriptions[group])
		if m == nil {
			t.Errorf("GroupDescriptions[%q] = %q lists no members in parentheses", group, cat.GroupDescriptions[group])
			continue
		}
		got, want := namesIn(m[1]), sortedCopy(cat.Groups[group])
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("GroupDescriptions[%q] names %v, Groups[%q] = %v", group, got, group, want)
		}
	}
}

// legacyNameRe matches the retired role IDs, group name and their
// capitalised prose forms. They survive only as aliases in internal/config
// (LegacyRoleAliases, LegacyGroupAliases) and in released history.
var legacyNameRe = regexp.MustCompile(`(?i)\bmelchor\b|\bkaji-audit\b|\bkaji-passes\b|\bkaji passes\b`)

// legacyPassFileTokens are the pre-rename audit pass names (file stem and
// Engram key suffix) that nerv-artifacts.md must name so an audit round in
// flight across the upgrade still compiles. They are the only legacy names the
// guard tolerates, and only in that file.
var legacyPassFileTokens = []string{"pass-melchor-round-N", "pass-kaji-audit-round-N"}

const legacyPassFallbackDoc = "skills/_shared/nerv-artifacts.md"

// stripAllowedLegacy removes the legacy pass-file tokens from a line of the
// one file that documents the fallback; every other file is checked verbatim.
func stripAllowedLegacy(path, line string) string {
	if path != legacyPassFallbackDoc {
		return line
	}
	for _, tok := range legacyPassFileTokens {
		line = strings.ReplaceAll(line, tok, "")
	}
	return line
}

// The plugin the user installs, the bench journeys and the integration guide
// name every renamed role by its new ID, so no orchestration prose launches a
// subagent type that no longer exists. docs/configuration.md is left out on
// purpose: it documents the legacy aliases.
func TestProse_NamesNoLegacyRoleIDs(t *testing.T) {
	check := func(fsys fs.FS, prefix string) {
		t.Helper()
		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(fsys, path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(data), "\n") {
				if prefix == "plugin/" {
					line = stripAllowedLegacy(path, line)
				}
				if m := legacyNameRe.FindString(line); m != "" {
					t.Errorf("%s%s:%d names legacy role %q", prefix, path, i+1, m)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	check(nerv.PluginFS(), "plugin/")
	check(os.DirFS("bench"), "bench/")
	check(fstest.MapFS{"integration.md": mustReadFile(t, "docs/integration.md")}, "docs/")
}

// An audit round started before the melchior/gendo rename keeps its pass
// files under the old names. nerv-artifacts.md states the read fallback once
// (both legacy file names, both legacy Engram keys, the canonical pass each
// counts as) and Kaji, who compiles the passes, points to it.
func TestAuditPasses_LegacyPassNameFallbackDocumented(t *testing.T) {
	artifacts := readPluginFile(t, legacyPassFallbackDoc)
	for _, want := range []string{
		"`pass-melchor-round-N.json`",
		"`pass-kaji-audit-round-N.json`",
		"`nerv/{change}/audit-pass-melchor-round-N`",
		"`nerv/{change}/audit-pass-kaji-audit-round-N`",
		"#### Legacy pass names",
	} {
		if !strings.Contains(artifacts, want) {
			t.Errorf("%s does not document the legacy pass fallback: missing %s", legacyPassFallbackDoc, want)
		}
	}
	if kaji := readPluginFile(t, "agents/kaji.md"); !strings.Contains(kaji, "Legacy pass names") {
		t.Errorf("agents/kaji.md does not point to the legacy pass-name fallback in nerv-artifacts.md")
	}
}

func readPluginFile(t *testing.T, path string) string {
	t.Helper()
	data, err := fs.ReadFile(nerv.PluginFS(), path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustReadFile(t *testing.T, path string) *fstest.MapFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: data}
}

var (
	agentDescriptionRe = regexp.MustCompile(`(?m)^description: (.*)$`)
	agentHeadingRe     = regexp.MustCompile(`(?m)^# (.*)$`)
)

// Every agent introduces itself by its full character name: the frontmatter
// description starts with "<display name>, " and the first heading with
// "<display name> — ", while the file name and name: stay the role ID.
func TestAgents_IntroduceTheirDisplayName(t *testing.T) {
	for _, role := range config.Roles().AllRoles {
		display := config.Roles().Info[role].DisplayName
		data, err := fs.ReadFile(nerv.PluginFS(), "agents/"+role+".md")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "\nname: "+role+"\n") {
			t.Errorf("agents/%s.md: frontmatter name is not the role ID %q", role, role)
		}
		if m := agentDescriptionRe.FindStringSubmatch(string(data)); m == nil || !strings.HasPrefix(m[1], display+", ") {
			t.Errorf("agents/%s.md: description must start with %q", role, display+", ")
		}
		if m := agentHeadingRe.FindStringSubmatch(string(data)); m == nil || !strings.HasPrefix(m[1], display+" — ") {
			t.Errorf("agents/%s.md: first heading must start with %q", role, display+" — ")
		}
	}
}
