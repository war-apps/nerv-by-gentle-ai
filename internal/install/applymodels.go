package install

import (
	"context"
	"fmt"
	"os"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
	"github.com/war-apps/nerv-gentle-ai/internal/configstore"
	"github.com/war-apps/nerv-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-gentle-ai/internal/models"
	"github.com/war-apps/nerv-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-gentle-ai/internal/version"
)

// ApplyModels applies the user-scope models: overrides (merged over the
// plugin's own committed defaults, resolving any from:<phase> entry
// against gentle-ai's state.json) to the cached agent frontmatter under
// the embedded plugin's own version. Mirrors -ApplyModels/-RefreshCache's
// shared Invoke-NervApplyModels tail in install.ps1, built entirely from
// packages P1a/P1b already ported and tested: config.ModelTableFromDocument
// does the default+override+from: merge that Resolve-NervModelAssignments
// and Merge-NervModelAssignments used to do by hand.
func ApplyModels(ctx context.Context, deps Deps) error {
	pluginVersion, err := version.PluginVersion(deps.FS)
	if err != nil {
		return fmt.Errorf("reading embedded plugin version: %w", err)
	}
	return applyModelsForVersion(deps, pluginVersion)
}

func applyModelsForVersion(deps Deps, pluginVersion string) error {
	fmt.Fprintf(deps.Stdout, "\n=== Applying model/effort assignments (nerv@nerv %s) ===\n", pluginVersion)

	p := paths.Resolve(deps.Home)

	agentsDir := p.CacheAgentsDir(pluginVersion)
	if _, err := os.Stat(agentsDir); os.IsNotExist(err) {
		fmt.Fprintf(deps.Stdout, "Warning: plugin cache agents directory not found: %s. Install/refresh the plugin first.\n", agentsDir)
		return nil
	}

	doc, _, err := (configstore.Store{}).Load(p.UserConfig)
	if err != nil {
		return fmt.Errorf("reading %s: %w", p.UserConfig, err)
	}

	defaults, err := models.PluginDefaults(deps.FS)
	if err != nil {
		return fmt.Errorf("reading plugin defaults: %w", err)
	}

	phaseAssignments, err := gentleai.PhaseAssignments(p.State)
	if err != nil {
		// A malformed state.json degrades to unresolved from:<phase>
		// display rather than failing the whole apply, matching
		// configure.Print's own tolerant handling of the same file.
		phaseAssignments = map[string]config.PhaseAssignment{}
	}

	rows := config.ModelTableFromDocument(doc, defaults, phaseAssignments)
	assignments := make(map[string]config.ModelOverride, len(rows))
	for _, r := range rows {
		assignments[r.Role] = config.ModelOverride{Model: r.Model, Effort: r.Effort}
	}

	summary, err := models.ApplyToDir(agentsDir, assignments)
	if err != nil {
		return fmt.Errorf("applying model assignments: %w", err)
	}

	fmt.Fprintf(deps.Stdout, "Model/effort apply summary: %d changed, %d already up to date, %d skipped.\n",
		summary.Changed, summary.UpToDate, summary.Skipped)
	return nil
}
