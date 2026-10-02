package wizard

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/models"
)

var (
	modelMenu  = menuOf(config.ModelAliases())
	effortMenu = menuOf(config.Efforts())

	resetRe = regexp.MustCompile(`(?i)^reset\s*(.*)$`)
)

// menuOf builds a "1"->tokens[0], "2"->tokens[1], ... menu-choice map, the
// shape runModelsSection's numbered prompts read from.
func menuOf(tokens []string) map[string]string {
	menu := make(map[string]string, len(tokens))
	for i, token := range tokens {
		menu[strconv.Itoa(i+1)] = token
	}
	return menu
}

// runModelsSection is Section 2: an offer to edit per-role model/effort
// overrides role by role or group by group ("magi", "pilots",
// "kaji-passes", "all"), "reset <target>" to clear one, "done" to finish,
// then one write through config.SetModelsBlock + configure.Store.Save.
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
	phaseNames := sortedPhaseNames(phaseAssignments)

	catalogue := config.Roles()
	overrides := config.ReadModelsOverrides(doc)

	for {
		table := config.ModelTable(defaults, overrides, phaseAssignments)
		printModelTable(out, table, paths.Config)

		roles, done := selectRoles(s, out, catalogue, overrides, table)
		if done {
			break
		}
		if roles == nil {
			continue
		}

		currentDisplay := currentModelDisplay(roles, table)

		newModel, newFrom, modelChoice, modelChosen, err := askModel(s, out, paths, phaseNames, phaseAssignments, currentDisplay)
		if err != nil {
			return false, err
		}

		newEffort, _, err := askEffort(s, currentDisplay, modelChoice, newFrom)
		if err != nil {
			return false, err
		}

		applyToOverrides(overrides, roles, modelChosen, modelChoice, newModel, newFrom, newEffort)
	}

	return writeModelsBlock(deps, s, out, paths, doc, original, overrides, hadExistingBlock)
}

// sortedPhaseNames returns phaseAssignments' keys in sorted order, the
// stable display/selection order the phase sub-menu (askModel's case "7")
// and its numbering read from.
func sortedPhaseNames(phaseAssignments map[string]config.PhaseAssignment) []string {
	names := make([]string, 0, len(phaseAssignments))
	for p := range phaseAssignments {
		names = append(names, p)
	}
	sort.Strings(names)
	return names
}

// currentModelDisplay is the "Enter keeps <this>" hint for a role
// selection: the single role's own "model/effort" when exactly one role
// is selected, or "(varies)" for a group covering more than one.
func currentModelDisplay(roles []string, table []config.ModelRow) string {
	if len(roles) != 1 {
		return "(varies)"
	}
	for _, row := range table {
		if row.Role == roles[0] {
			return row.Model + "/" + row.Effort
		}
	}
	return "(none)"
}

// selectRoles reads one role-selection prompt — a role name/number, a
// group shortcut, "reset <target>", or "done" — and resolves it: done is
// true once the user answers "done"; a reset target clears those roles'
// overrides directly and returns (nil, false) to loop again; anything
// else resolves to the roles a model/effort edit should apply to, or nil
// (an unknown-target message already printed) to loop again.
func selectRoles(s *session, out io.Writer, catalogue config.RoleCatalogue, overrides map[string]config.ModelOverride, table []config.ModelRow) (roles []string, done bool) {
	numberMap := make(map[string]string, len(table))
	for i, row := range table {
		numberMap[strconv.Itoa(i+1)] = row.Role
	}

	roleAnswer := strings.TrimSpace(s.promptExhausted(
		`Role (name, number, magi | pilots | kaji-passes | all), "reset" to clear an override, "done" to finish:`, "done"))

	if roleAnswer == "" {
		return nil, false
	}
	if strings.EqualFold(roleAnswer, "done") {
		return nil, true
	}

	if m := resetRe.FindStringSubmatch(roleAnswer); m != nil {
		target := strings.TrimSpace(m[1])
		if target == "" {
			target = strings.TrimSpace(s.promptExhausted("Reset which role or group?", "done"))
		}
		if target == "" || strings.EqualFold(target, "done") {
			return nil, false
		}
		resetRoles := config.ResolveRoleTarget(target, catalogue.AllRoles, catalogue.Groups, numberMap)
		if resetRoles == nil {
			printUnknownRoleTarget(out, target, catalogue.AllRoles)
			return nil, false
		}
		for _, r := range resetRoles {
			delete(overrides, r)
		}
		return nil, false
	}

	roles = config.ResolveRoleTarget(roleAnswer, catalogue.AllRoles, catalogue.Groups, numberMap)
	if roles == nil {
		printUnknownRoleTarget(out, roleAnswer, catalogue.AllRoles)
		return nil, false
	}
	return roles, false
}

// askModel prompts the model choice for the current selection
// (currentDisplay is its "Enter keeps ..." hint), resolving a custom
// model id or gentle-ai phase sub-prompt as needed. chosen is false when
// the answer was blank (keep current model unchanged); modelChoice is the
// raw menu answer ("" when blank), which askEffort and applyToOverrides
// also need (case "7" drives the effort prompt's "inherited" wording and
// From-vs-Model application).
func askModel(s *session, out io.Writer, paths configure.Paths, phaseNames []string, phaseAssignments map[string]config.PhaseAssignment, currentDisplay string) (newModel, newFrom, modelChoice string, chosen bool, err error) {
	modelChoice, chosen, err = s.menuChoice(
		fmt.Sprintf("Model (1 sonnet, 2 opus, 3 haiku, 4 fable, 5 inherit, 6 custom id, 7 from gentle-ai phase, Enter keeps %s):", currentDisplay),
		[]string{"1", "2", "3", "4", "5", "6", "7"})
	if err != nil || !chosen {
		return "", "", modelChoice, false, err
	}

	switch modelChoice {
	case "6":
		for {
			customID, err := s.promptRequired("Custom model id (claude-...):")
			if err != nil {
				return "", "", modelChoice, false, err
			}
			if customID == "" {
				// blank keeps the current model unchanged, mirroring
				// every other model choice's blank-answer semantics.
				return "", "", modelChoice, false, nil
			}
			if config.IsCustomModelID(customID) {
				return customID, "", modelChoice, true, nil
			}
			fmt.Fprintln(out, "Invalid model id; must match ^claude-.+$")
		}
	case "7":
		if len(phaseNames) == 0 {
			fmt.Fprintf(out, "No gentle-ai phases found in %s.\n", paths.State)
			return "", "", modelChoice, true, nil
		}
		fmt.Fprintln(out, "Phases:")
		valid := make([]string, len(phaseNames))
		for i, p := range phaseNames {
			pa := phaseAssignments[p]
			fmt.Fprintf(out, "  %d) %s (%s/%s)\n", i+1, p, pa.Model, pa.Effort)
			valid[i] = strconv.Itoa(i + 1)
		}
		idx, ok, err := s.menuChoice("Phase number:", valid)
		if err != nil {
			return "", "", modelChoice, false, err
		}
		if !ok {
			return "", "", modelChoice, true, nil
		}
		n, _ := strconv.Atoi(idx)
		return "", phaseNames[n-1], modelChoice, true, nil
	default:
		return modelMenu[modelChoice], "", modelChoice, true, nil
	}
}

// askEffort prompts the effort choice, using currentDisplay for the
// "Enter keeps ..." label unless askModel just resolved a gentle-ai phase
// (modelChoice=="7" with a non-empty newFrom), in which case the label
// says "Enter keeps inherited" instead. chosen is false when the answer
// was blank (keep current effort unchanged).
func askEffort(s *session, currentDisplay, modelChoice, newFrom string) (newEffort string, chosen bool, err error) {
	label := fmt.Sprintf("Effort (1 low, 2 medium, 3 high, 4 xhigh, 5 max, Enter keeps %s):", currentDisplay)
	if modelChoice == "7" && newFrom != "" {
		label = "Effort (1 low, 2 medium, 3 high, 4 xhigh, 5 max, Enter keeps inherited):"
	}
	effortChoice, chosen, err := s.menuChoice(label, []string{"1", "2", "3", "4", "5"})
	if err != nil || !chosen {
		return "", false, err
	}
	return effortMenu[effortChoice], true, nil
}

// applyToOverrides applies one resolved model/effort answer onto every
// role in roles, deleting a role's override entirely once it goes back to
// the zero value (matching config.ModelOverride's own "empty means
// absent" convention).
func applyToOverrides(overrides map[string]config.ModelOverride, roles []string, modelChosen bool, modelChoice, newModel, newFrom, newEffort string) {
	for _, r := range roles {
		entry := overrides[r]

		if modelChosen {
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

// writeModelsBlock renders overrides into a models: block (or clears an
// existing one), asks for confirmation, and writes it through
// config.SetModelsBlock + configure.Store.Save. The returned bool is
// runModelsSection's own "did this section change anything" report.
func writeModelsBlock(deps Deps, s *session, out io.Writer, paths configure.Paths, doc *config.Document, original []byte, overrides map[string]config.ModelOverride, hadExistingBlock bool) (bool, error) {
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
