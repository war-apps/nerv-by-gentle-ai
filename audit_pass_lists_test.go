package nerv_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// auditPassNames derives the five Phase 3 audit passes from the role
// catalogue: the three MAGI (audit mode) plus the kaji-passes group without
// the compiler (kaji) and the detached refuter (kaji-refuter), which run
// after the parallel batch rather than inside it.
func auditPassNames(t *testing.T) []string {
	t.Helper()
	groups := config.Roles().Groups
	names := append([]string(nil), groups["magi"]...)
	for _, role := range groups["kaji-passes"] {
		if role == "kaji" || role == "kaji-refuter" {
			continue
		}
		names = append(names, role)
	}
	if len(names) != 5 {
		t.Fatalf("derived %d audit passes from config.Roles() (%v), want 5", len(names), names)
	}
	return names
}

// auditPassList names one prose region that must list every audit pass.
type auditPassList struct {
	file   string
	region string
	// anchor is a substring of the region's first line.
	anchor string
	// row extracts only the agents cell of the anchor line (a table row):
	// the cell right after the step label. Other cells (inputs, output)
	// also name passes, so matching the whole row would let a pass drop out
	// of the agents column unnoticed. Otherwise the region runs from the
	// anchor line to the next blank line (one paragraph).
	row bool
}

var auditPassLists = []auditPassList{
	{
		file:   "plugin/skills/nerv-orchestrator/references/pipeline-full.md",
		region: "Pass batch and JSON gatekeeping paragraph",
		anchor: "**Pass batch and JSON gatekeeping.**",
	},
	{
		file:   "plugin/skills/nerv-orchestrator/references/pipeline-full.md",
		region: "step 14 table row agents cell",
		anchor: "| 14.",
		row:    true,
	},
	{
		file:   "plugin/skills/_shared/nerv-artifacts.md",
		region: "audit pass JSON output contract launch paragraph",
		anchor: "Each of `nerv:melchor`",
	},
	{
		file:   "plugin/agents/kaji.md",
		region: "Role contract compile paragraph",
		anchor: "Kaji has one mode: compile.",
	},
}

// proseRegion returns the region of data that starts at the line containing
// anchor: the agents cell of that line for a table row, or the paragraph up
// to the next blank line.
func proseRegion(t *testing.T, data string, list auditPassList) string {
	t.Helper()
	lines := strings.Split(data, "\n")
	for i, line := range lines {
		if !strings.Contains(line, list.anchor) {
			continue
		}
		if list.row {
			// "| step | agents | inputs | ..." splits into "", step, agents, ...
			cells := strings.Split(line, "|")
			if len(cells) < 3 {
				t.Fatalf("%s: %s has no agents cell after the step label: %q", list.file, list.region, line)
			}
			return cells[2]
		}
		end := i
		for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
			end++
		}
		return strings.Join(lines[i:end], "\n")
	}
	t.Fatalf("%s: anchor %q for %s not found", list.file, list.anchor, list.region)
	return ""
}

func TestAuditPassLists_NameAllFivePasses(t *testing.T) {
	passes := auditPassNames(t)
	for _, list := range auditPassLists {
		data, err := os.ReadFile(list.file)
		if err != nil {
			t.Fatal(err)
		}
		region := proseRegion(t, string(data), list)
		for _, pass := range passes {
			// Bound the name so `kaji-coverage` never satisfies a bare `kaji`
			// and a pass name never matches inside a longer identifier.
			re := regexp.MustCompile(`(^|[^a-z-])` + regexp.QuoteMeta(pass) + `([^a-z-]|$)`)
			if !re.MatchString(region) {
				t.Errorf("%s (%s): audit pass %q missing from the launch list", list.file, list.region, pass)
			}
		}
	}
}
