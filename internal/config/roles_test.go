package config_test

import (
	"io/fs"
	"sort"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// wantEquivalents is the user-approved role -> gentle-ai v4 agent table (the jd-judge
// and native review agents that still exist in v4). An empty value means
// the role has no equivalent.
var wantEquivalents = map[string]string{
	"misato": "", "ritsuko": "", "hyuga": "",
	"melchor": "jd-judge-b", "balthasar": "jd-judge-a", "casper": "jd-judge-a",
	"fuyutsuki": "",
	"kaworu":    "", "shinji": "", "asuka": "", "rei": "", "toji": "",
	"maya": "", "kaji": "", "kaji-security": "review-risk",
	"kaji-coverage": "review-reliability", "kaji-refuter": "review-refuter", "aoba": "",
}

func TestRoles_EveryRoleHasPurposeAndEquivalent(t *testing.T) {
	cat := config.Roles()
	for _, role := range cat.AllRoles {
		info, ok := cat.Info[role]
		if !ok {
			t.Errorf("role %q has no Info entry", role)
			continue
		}
		if strings.TrimSpace(info.Purpose) == "" {
			t.Errorf("role %q has an empty Purpose", role)
		}
		if strings.Contains(info.Purpose, "\n") {
			t.Errorf("role %q Purpose must be one line: %q", role, info.Purpose)
		}
		want, known := wantEquivalents[role]
		if !known {
			t.Errorf("role %q missing from the expected equivalence table", role)
			continue
		}
		if info.GentleAIEquivalent != want {
			t.Errorf("role %q equivalent = %q, want %q", role, info.GentleAIEquivalent, want)
		}
	}
	if len(cat.Info) != len(cat.AllRoles) {
		t.Errorf("Info has %d entries, AllRoles has %d", len(cat.Info), len(cat.AllRoles))
	}
}

func TestRoles_EveryGroupHasADescription(t *testing.T) {
	cat := config.Roles()
	for group := range cat.Groups {
		if strings.TrimSpace(cat.GroupDescriptions[group]) == "" {
			t.Errorf("group %q has no description", group)
		}
	}
	if len(cat.GroupDescriptions) != len(cat.Groups) {
		t.Errorf("GroupDescriptions has %d entries, Groups has %d", len(cat.GroupDescriptions), len(cat.Groups))
	}
}

func TestRoles_GroupMembership(t *testing.T) {
	groups := config.Roles().Groups
	want := map[string][]string{
		"magi":        {"balthasar", "casper", "melchor"},
		"pilots":      {"asuka", "kaworu", "rei", "shinji", "toji"},
		"kaji-passes": {"kaji", "kaji-coverage", "kaji-refuter", "kaji-security"},
	}
	for name, members := range want {
		got := append([]string(nil), groups[name]...)
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(members, ",") {
			t.Errorf("group %q = %v, want %v", name, got, members)
		}
	}
}

// The catalogue and the plugin's agents/*.md must describe the same roles.
func TestRoles_CatalogueMatchesPluginAgents(t *testing.T) {
	entries, err := fs.ReadDir(nerv.PluginFS(), "agents")
	if err != nil {
		t.Fatal(err)
	}
	var agents []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".md"); ok {
			agents = append(agents, name)
		}
	}
	catalogue := append([]string(nil), config.Roles().AllRoles...)
	sort.Strings(agents)
	sort.Strings(catalogue)
	if strings.Join(agents, ",") != strings.Join(catalogue, ",") {
		t.Errorf("catalogue roles != plugin agents\ncatalogue: %v\nagents:    %v", catalogue, agents)
	}
}
