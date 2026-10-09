package nerv_test

import (
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

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
	group, purpose, equivalent, defaults string
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
		if len(cells) != 5 {
			t.Fatalf("docs roles table row has %d cells, want 5: %q", len(cells), line)
		}
		role := stripTicks(cells[0])
		if _, dup := rows[role]; dup {
			t.Errorf("docs roles table lists role %q twice", role)
		}
		rows[role] = docsRoleRow{
			group:      stripTicks(cells[1]),
			purpose:    strings.TrimSpace(cells[2]),
			equivalent: equivalentsCell(cells[3]),
			defaults:   strings.TrimSpace(cells[4]),
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

// The plugin the user installs names every renamed role by its new ID, so
// no orchestration prose launches a subagent type that no longer exists.
func TestPlugin_NamesNoLegacyRoleIDs(t *testing.T) {
	err := fs.WalkDir(nerv.PluginFS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(nerv.PluginFS(), path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if m := legacyNameRe.FindString(line); m != "" {
				t.Errorf("plugin/%s:%d names legacy role %q", path, i+1, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
