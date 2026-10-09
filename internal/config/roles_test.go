package config_test

import (
	"io/fs"
	"regexp"
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
	"melchior":  {"jd-judge-b", "review-risk"},
	"balthasar": {"jd-judge-a", "review-readability"},
	"casper":    nil,
	"fuyutsuki": {"review-refuter"},
	"kaworu":    {"jd-fix-agent"}, "shinji": {"jd-fix-agent"}, "asuka": {"jd-fix-agent"},
	"rei": {"jd-fix-agent"}, "toji": {"jd-fix-agent"},
	"maya": nil, "kaji": nil,
	"gendo": {"review-reliability", "review-resilience"},
	"aoba":  nil,
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
	"melchior": "jd-judge-b", "balthasar": "jd-judge-a", "casper": "",
	"fuyutsuki": "",
	"kaworu":    "", "shinji": "", "asuka": "", "rei": "", "toji": "",
	"maya": "", "kaji": "",
	"gendo": "", "aoba": "",
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
		"magi":         {"balthasar", "casper", "melchior"},
		"pilots":       {"asuka", "kaworu", "rei", "shinji", "toji"},
		"audit-passes": {"gendo", "kaji"},
	}
	for name, members := range want {
		got := append([]string(nil), groups[name]...)
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(members, ",") {
			t.Errorf("group %q = %v, want %v", name, got, members)
		}
	}
}

// wantDisplayNames is the user-approved role ID -> full character name
// table. IDs stay short lowercase slugs; the display name is for people.
var wantDisplayNames = map[string]string{
	"misato":    "Misato Katsuragi",
	"ritsuko":   "Ritsuko Akagi",
	"hyuga":     "Makoto Hyuga",
	"melchior":  "Melchior-Magi 1",
	"balthasar": "Balthasar-Magi 2",
	"casper":    "Casper-Magi 3",
	"fuyutsuki": "Kōzō Fuyutsuki",
	"kaworu":    "Kaworu Nagisa",
	"shinji":    "Shinji Ikari",
	"asuka":     "Asuka Langley Sohryu",
	"rei":       "Rei Ayanami",
	"toji":      "Tōji Suzuhara",
	"maya":      "Maya Ibuki",
	"kaji":      "Ryoji Kaji",
	"gendo":     "Gendo Ikari",
	"aoba":      "Shigeru Aoba",
}

func TestRoles_EveryRoleHasItsDisplayName(t *testing.T) {
	cat := config.Roles()
	for _, role := range cat.AllRoles {
		want, known := wantDisplayNames[role]
		if !known {
			t.Errorf("role %q missing from the expected display-name table", role)
			continue
		}
		if got := cat.Info[role].DisplayName; got != want {
			t.Errorf("role %q DisplayName = %q, want %q", role, got, want)
		}
	}
	if len(wantDisplayNames) != len(cat.AllRoles) {
		t.Errorf("display-name table has %d entries, AllRoles has %d", len(wantDisplayNames), len(cat.AllRoles))
	}
}

// Every role ID stays a short lowercase slug usable as a Claude Code agent
// name, a nerv:<id> subagent type and a nerv.yaml models: key.
func TestRoles_IDsAreLowercaseSlugs(t *testing.T) {
	slug := regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)
	for _, role := range config.Roles().AllRoles {
		if !slug.MatchString(role) {
			t.Errorf("role ID %q is not a lowercase slug", role)
		}
	}
}

// The renamed roles and group keep their old names as legacy aliases.
func TestCanonicalRole(t *testing.T) {
	cases := []struct{ in, want string }{
		{"melchor", "melchior"},
		{"kaji-audit", "gendo"},
		{"melchior", "melchior"},
		{"gendo", "gendo"},
		{"misato", "misato"},
		// Unknown names pass through unchanged so callers keep reporting them.
		{"not-a-role", "not-a-role"},
	}
	for _, tc := range cases {
		if got := config.CanonicalRole(tc.in); got != tc.want {
			t.Errorf("CanonicalRole(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLegacyAliases_PointAtCatalogueEntries(t *testing.T) {
	cat := config.Roles()
	for old, id := range config.LegacyRoleAliases() {
		if slices.Contains(cat.AllRoles, old) {
			t.Errorf("legacy alias %q is still a catalogue role", old)
		}
		if !slices.Contains(cat.AllRoles, id) {
			t.Errorf("legacy alias %q -> %q, which is not a catalogue role", old, id)
		}
	}
	for old, group := range config.LegacyGroupAliases() {
		if _, ok := cat.Groups[old]; ok {
			t.Errorf("legacy group alias %q is still a catalogue group", old)
		}
		if _, ok := cat.Groups[group]; !ok {
			t.Errorf("legacy group alias %q -> %q, which is not a catalogue group", old, group)
		}
	}
	if got := config.LegacyRoleAliases(); got["melchor"] != "melchior" || got["kaji-audit"] != "gendo" || len(got) != 2 {
		t.Errorf("LegacyRoleAliases() = %v, want melchor->melchior, kaji-audit->gendo", got)
	}
	if got := config.LegacyGroupAliases(); got["kaji-passes"] != "audit-passes" || len(got) != 1 {
		t.Errorf("LegacyGroupAliases() = %v, want kaji-passes->audit-passes", got)
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
