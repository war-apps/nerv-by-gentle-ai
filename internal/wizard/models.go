package wizard

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
	"github.com/war-apps/nerv-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-gentle-ai/internal/models"
)

var (
	modelMenu  = map[string]string{"1": "sonnet", "2": "opus", "3": "haiku", "4": "fable", "5": "inherit"}
	effortMenu = map[string]string{"1": "low", "2": "medium", "3": "high", "4": "xhigh", "5": "max"}

	resetRe         = regexp.MustCompile(`(?i)^reset\s*(.*)$`)
	customModelIDRe = regexp.MustCompile(`^claude-.+$`)
)

// runModelsSection is Section 2 (configure.ps1's Section 2 launching
// configure-models.ps1, folded in directly): an offer to edit per-role
// model/effort overrides role by role or group by group ("magi", "pilots",
// "kaji-passes", "all"), "reset <target>" to clear one, "done" to finish,
// then one write through config.SetModelsBlock + configure.Store.Save.
// Mirrors configure-models.ps1's interactive body (~480-688).
func runModelsSection(deps Deps, paths configure.Paths, s *session, out io.Writer) (bool, error) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- Models ---")
	configureNow, err := s.yesNo("Configure per-role model and effort now?", false)
	if err != nil {
		return false, err
	}
	if !configureNow {
		return false, nil
	}

	doc, _, err := (configure.Store{}).Load(paths.Config)
	if err != nil {
		return false, err
	}
	original := []byte(doc.String())
	hadExistingBlock := doc.KeyExists("models")

	defaults, err := models.PluginDefaults(deps.FS)
	if err != nil {
		return false, err
	}
	phaseAssignments, err := gentleai.PhaseAssignments(paths.State)
	if err != nil {
		phaseAssignments = map[string]config.PhaseAssignment{}
	}
	phaseNames := make([]string, 0, len(phaseAssignments))
	for p := range phaseAssignments {
		phaseNames = append(phaseNames, p)
	}
	sort.Strings(phaseNames)

	catalogue := config.Roles()
	overrides := config.ReadModelsOverrides(doc)

	for {
		table := config.ModelTable(defaults, overrides, phaseAssignments)
		printModelTable(out, table, paths.Config)

		numberMap := make(map[string]string, len(table))
		for i, row := range table {
			numberMap[strconv.Itoa(i+1)] = row.Role
		}

		roleAnswer := strings.TrimSpace(s.promptExhausted(
			`Role (name, number, magi | pilots | kaji-passes | all), "reset" to clear an override, "done" to finish:`, "done"))

		if roleAnswer == "" {
			continue
		}
		if strings.EqualFold(roleAnswer, "done") {
			break
		}

		if m := resetRe.FindStringSubmatch(roleAnswer); m != nil {
			target := strings.TrimSpace(m[1])
			if target == "" {
				target = strings.TrimSpace(s.promptExhausted("Reset which role or group?", "done"))
			}
			if target == "" || strings.EqualFold(target, "done") {
				continue
			}
			resetRoles := config.ResolveRoleTarget(target, catalogue.AllRoles, catalogue.Groups, numberMap)
			if resetRoles == nil {
				printUnknownRoleTarget(out, target, catalogue.AllRoles)
				continue
			}
			for _, r := range resetRoles {
				delete(overrides, r)
			}
			continue
		}

		roles := config.ResolveRoleTarget(roleAnswer, catalogue.AllRoles, catalogue.Groups, numberMap)
		if roles == nil {
			printUnknownRoleTarget(out, roleAnswer, catalogue.AllRoles)
			continue
		}

		currentDisplay := "(varies)"
		if len(roles) == 1 {
			currentDisplay = "(none)"
			for _, row := range table {
				if row.Role == roles[0] {
					currentDisplay = row.Model + "/" + row.Effort
					break
				}
			}
		}

		modelChoice, chosen, err := s.menuChoice(
			fmt.Sprintf("Model (1 sonnet, 2 opus, 3 haiku, 4 fable, 5 inherit, 6 custom id, 7 from gentle-ai phase, Enter keeps %s):", currentDisplay),
			[]string{"1", "2", "3", "4", "5", "6", "7"})
		if err != nil {
			return false, err
		}

		var newModel, newFrom string
		if chosen {
			switch modelChoice {
			case "6":
				for {
					customID, err := s.promptRequired("Custom model id (claude-...):")
					if err != nil {
						return false, err
					}
					if customID == "" {
						// blank keeps the current model unchanged, mirroring
						// every other model choice's blank-answer semantics.
						chosen = false
						break
					}
					if customModelIDRe.MatchString(customID) {
						newModel = customID
						break
					}
					fmt.Fprintln(out, "Invalid model id; must match ^claude-.+$")
				}
			case "7":
				if len(phaseNames) == 0 {
					fmt.Fprintf(out, "No gentle-ai phases found in %s.\n", paths.State)
				} else {
					fmt.Fprintln(out, "Phases:")
					valid := make([]string, len(phaseNames))
					for i, p := range phaseNames {
						pa := phaseAssignments[p]
						fmt.Fprintf(out, "  %d) %s (%s/%s)\n", i+1, p, pa.Model, pa.Effort)
						valid[i] = strconv.Itoa(i + 1)
					}
					idx, ok, err := s.menuChoice("Phase number:", valid)
					if err != nil {
						return false, err
					}
					if ok {
						n, _ := strconv.Atoi(idx)
						newFrom = phaseNames[n-1]
					}
				}
			default:
				newModel = modelMenu[modelChoice]
			}
		}

		effortLabel := fmt.Sprintf("Effort (1 low, 2 medium, 3 high, 4 xhigh, 5 max, Enter keeps %s):", currentDisplay)
		if modelChoice == "7" && newFrom != "" {
			effortLabel = "Effort (1 low, 2 medium, 3 high, 4 xhigh, 5 max, Enter keeps inherited):"
		}
		effortChoice, effortChosen, err := s.menuChoice(effortLabel, []string{"1", "2", "3", "4", "5"})
		if err != nil {
			return false, err
		}
		var newEffort string
		if effortChosen {
			newEffort = effortMenu[effortChoice]
		}

		for _, r := range roles {
			entry := overrides[r]

			if chosen {
				if modelChoice == "7" {
					if newFrom != "" {
						entry.From = newFrom
						entry.Model = ""
					}
				} else {
					entry.Model = newModel
					entry.From = ""
				}
			}

			if modelChoice == "7" && newFrom != "" {
				entry.Effort = newEffort
			} else if newEffort != "" {
				entry.Effort = newEffort
			}

			if entry == (config.ModelOverride{}) {
				delete(overrides, r)
			} else {
				overrides[r] = entry
			}
		}
	}

	blockText := ""
	if len(overrides) > 0 {
		blockText = config.FormatModelsBlock(overrides)
	}

	if blockText == "" && !hadExistingBlock {
		fmt.Fprintln(out, "No overrides configured; nothing to write.")
		return false, nil
	}

	fmt.Fprintln(out)
	if blockText != "" {
		fmt.Fprintf(out, "The following models: block will be written to %s:\n\n%s\n\n", paths.Config, blockText)
	} else {
		fmt.Fprintf(out, "No overrides left; the existing models: block will be removed from %s.\n", paths.Config)
	}

	write, err := s.yesNo(fmt.Sprintf("Write to %s?", paths.Config), true)
	if err != nil {
		return false, err
	}
	if !write {
		fmt.Fprintln(out, "Aborted; no changes written.")
		return false, nil
	}

	config.SetModelsBlock(doc, blockText)
	written, backup, err := (configure.Store{}).Save(paths.Config, doc, original, deps.Now())
	if err != nil {
		return false, err
	}
	if backup != "" {
		fmt.Fprintf(out, "Backup written: %s\n", backup)
	}
	if written {
		fmt.Fprintf(out, "Written: %s\n", paths.Config)
	}
	return written, nil
}

func printModelTable(out io.Writer, table []config.ModelRow, configPath string) {
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Config: %s\n", configPath)
	fmt.Fprintf(out, "%-16s %-16s %-8s %s\n", "ROLE", "MODEL", "EFFORT", "SOURCE")
	for i, row := range table {
		fmt.Fprintf(out, "%2d) %-16s %-16s %-8s %s\n", i+1, row.Role, row.Model, row.Effort, row.Source)
	}
	fmt.Fprintln(out)
}

func printUnknownRoleTarget(out io.Writer, target string, allRoles []string) {
	fmt.Fprintf(out, "Unknown role/group/number: %s. Valid roles: %s; groups: magi, pilots, kaji-passes, all.\n",
		target, strings.Join(allRoles, ", "))
}
