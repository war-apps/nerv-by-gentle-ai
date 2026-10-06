package config_test

import (
	"io/fs"
	"slices"
	"sort"
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// wantEquivalents is the user-approved role -> gentle-ai v4 agents table
// (the jd-* and native review-* agents). A nil value means the role has no
// equivalent. The pilots claim jd-fix-agent because fix routing goes through
// the owning pilot, with kaworu writing the RED test first.
var wantEquivalents = map[string][]string{
	"misato": nil, "ritsuko": nil, "hyuga": nil,
	"melchor":   {"jd-judge-b"},
	"balthasar": {"jd-judge-a", "review-readability"},
	"casper":    {"jd-judge-a"},
	"fuyutsuki": nil,
	"kaworu":    {"jd-fix-agent"}, "shinji": {"jd-fix-agent"}, "asuka": {"jd-fix-agent"},
	"rei": {"jd-fix-agent"}, "toji": {"jd-fix-agent"},
	"maya": nil, "kaji": nil,
	"kaji-security":   {"review-risk"},
	"kaji-coverage":   {"review-reliability"},
	"kaji-resilience": {"review-resilience"},
	"kaji-refuter":    {"review-refuter"},
	"aoba":            nil,
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
		if !slices.Equal(info.GentleAIEquivalents, want) {
			t.Errorf("role %q equivalents = %q, want %q", role, info.GentleAIEquivalents, want)
		}
	}
	if len(cat.Info) != len(cat.AllRoles) {
		t.Errorf("Info has %d entries, AllRoles has %d", len(cat.Info), len(cat.AllRoles))
	}
}

// wantFromPhases is the role -> gentle-ai claude_phase_assignments key the
// role suggests as from:<phase>. Unlike wantEquivalents it only holds keys
// that exist in gentle-ai 4.x state; an empty value means no suggestion.
var wantFromPhases = map[string]string{
	"misato": "", "ritsuko": "", "hyuga": "",
	"melchor": "jd-judge-b", "balthasar": "jd-judge-a", "casper": "jd-judge-a",
	"fuyutsuki": "",
	"kaworu":    "", "shinji": "", "asuka": "", "rei": "", "toji": "",
	"maya": "", "kaji": "", "kaji-security": "",
	"kaji-coverage": "", "kaji-resilience": "", "kaji-refuter": "", "aoba": "",
}

// v4PhaseKeys are the claude_phase_assignments keys that still have an
// agent in gentle-ai 4.x, the only values usable as from:<phase>.
var v4PhaseKeys = []string{"jd-judge-a", "jd-judge-b", "jd-fix-agent"}

func TestRoles_FromPhaseIsAGentleAIPhaseKey(t *testing.T) {
	cat := config.Roles()
	for _, role := range cat.AllRoles {
		info := cat.Info[role]
		want, known := wantFromPhases[role]
		if !known {
			t.Errorf("role %q missing from the expected from-phase table", role)
			continue
		}
		if info.FromPhase != want {
			t.Errorf("role %q FromPhase = %q, want %q", role, info.FromPhase, want)
		}
		if info.FromPhase != "" && !slices.Contains(v4PhaseKeys, info.FromPhase) {
			t.Errorf("role %q FromPhase %q is not a gentle-ai 4.x phase key %v", role, info.FromPhase, v4PhaseKeys)
		}
		if strings.HasPrefix(info.FromPhase, "review-") {
			t.Errorf("role %q FromPhase %q is a native review agent, not a phase key", role, info.FromPhase)
		}
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
		"kaji-passes": {"kaji", "kaji-coverage", "kaji-refuter", "kaji-resilience", "kaji-security"},
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
