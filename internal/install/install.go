package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/war-apps/nerv-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-gentle-ai/internal/engram"
	"github.com/war-apps/nerv-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-gentle-ai/internal/plugin"
	"github.com/war-apps/nerv-gentle-ai/internal/skills"
	"github.com/war-apps/nerv-gentle-ai/internal/version"
)

const pluginID = "nerv@nerv"
const marketplaceName = "nerv"

// Install runs the whole "nerv install" use case: the gentle-ai preflight,
// materializing the embedded plugin under <home>/.nerv/marketplace,
// registering that directory marketplace and enabling nerv@nerv in
// settings.json, refreshing the plugin cache through "claude plugin
// uninstall/install" and verifying it against the embedded version,
// applying model/effort assignments to the cached agents, ensuring the
// Engram "nerv" knowledge base, installing skills (unless opts.NoSkills),
// and printing the configure hint (unless opts.NoConfigure). Every step's
// progress is written to deps.Stdout as it runs. Mirrors install.ps1's
// main body under the embedded-plugin model (see this package's doc
// comment).
func Install(ctx context.Context, deps Deps, opts Options) error {
	if err := preflightGentleAI(ctx, deps, opts.RequireGentleAI); err != nil {
		return err
	}

	marketplaceDir := filepath.Join(deps.Home, ".nerv", "marketplace")
	matResult, err := plugin.Materialize(deps.FS, marketplaceDir)
	if err != nil {
		return fmt.Errorf("materializing plugin: %w", err)
	}
	fmt.Fprintf(deps.Stdout, "Plugin materialized at %s (%d written, %d unchanged, %d removed)\n",
		marketplaceDir, len(matResult.Written), len(matResult.Unchanged), len(matResult.Removed))

	if err := registerSettings(deps, marketplaceDir); err != nil {
		return err
	}

	pluginVersion, err := version.PluginVersion(deps.FS)
	if err != nil {
		return fmt.Errorf("reading embedded plugin version: %w", err)
	}
	if err := refreshCache(ctx, deps, pluginVersion); err != nil {
		return err
	}

	if err := applyModelsForVersion(deps, pluginVersion); err != nil {
		return err
	}

	ensureEngram(ctx, deps)

	if !opts.NoSkills {
		installSkills(ctx, deps)
	}

	if !opts.NoConfigure {
		fmt.Fprintln(deps.Stdout, "\nRun `nerv configure` to set up NERV.")
	}

	fmt.Fprintln(deps.Stdout, "\nRestart Claude Code for the change to take effect.")
	return nil
}

func preflightGentleAI(ctx context.Context, deps Deps, require bool) error {
	preflight := gentleai.CheckPreflight(ctx, deps.Runner)

	switch {
	case !preflight.Found:
		fmt.Fprintln(deps.Stdout, "Warning: gentle-ai not found on PATH; NERV requires gentle-ai 3.x (https://github.com/Gentleman-Programming/gentle-ai)")
		if require {
			return &RefusalError{Err: errors.New("gentle-ai not found on PATH")}
		}
	case preflight.Version == "":
		fmt.Fprintln(deps.Stdout, "Warning: gentle-ai --version returned an unparseable value")
		if require {
			return &RefusalError{Err: errors.New("gentle-ai --version returned an unparseable value")}
		}
	case !preflight.OK:
		fmt.Fprintf(deps.Stdout, "Warning: NERV requires gentle-ai 3.x; found %s\n", preflight.Version)
		if require {
			return &RefusalError{Err: fmt.Errorf("NERV requires gentle-ai 3.x; found %s", preflight.Version)}
		}
	default:
		fmt.Fprintf(deps.Stdout, "gentle-ai version : %s (tested against 3.7.0)\n", preflight.Version)
	}
	return nil
}

func settingsPath(deps Deps) string {
	if deps.SettingsPath != "" {
		return deps.SettingsPath
	}
	return filepath.Join(deps.Home, ".claude", "settings.json")
}

// loadOrInitSettings loads path, treating a missing file as an empty
// document (settings.json does not necessarily exist before the first
// install), and returns its raw bytes (nil when the file did not exist)
// alongside the parsed *claude.Settings so the caller can diff against
// them via claude.SaveSettings.
func loadOrInitSettings(path string) (previous []byte, settings *claude.Settings, err error) {
	s, err := claude.LoadSettings(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, claude.NewSettings(), nil
		}
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	previous, err = s.Bytes()
	if err != nil {
		return nil, nil, err
	}
	return previous, s, nil
}

func registerSettings(deps Deps, marketplaceDir string) error {
	path := settingsPath(deps)
	previous, settings, err := loadOrInitSettings(path)
	if err != nil {
		return err
	}

	changedMarketplace := settings.RegisterMarketplace(marketplaceName, marketplaceDir)
	changedPlugin := settings.EnablePlugin(pluginID)

	if !changedMarketplace && !changedPlugin {
		fmt.Fprintln(deps.Stdout, "settings.json already up to date.")
		return nil
	}

	written, backup, err := claude.SaveSettings(path, settings, previous, deps.Now())
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if written {
		if backup != "" {
			fmt.Fprintf(deps.Stdout, "Backup written: %s\n", backup)
		}
		fmt.Fprintln(deps.Stdout, "settings.json updated and verified.")
	}
	return nil
}

// RefreshCache re-registers nerv@nerv in the Claude Code plugin cache
// ("claude plugin uninstall/install") and verifies the cached version
// matches the embedded plugin version. Exported for internal/wizard's
// closing "apply and refresh" step, wrapping the same tail Install itself
// runs after registering settings.json.
func RefreshCache(ctx context.Context, deps Deps) error {
	pluginVersion, err := version.PluginVersion(deps.FS)
	if err != nil {
		return fmt.Errorf("reading embedded plugin version: %w", err)
	}
	return refreshCache(ctx, deps, pluginVersion)
}

func refreshCache(ctx context.Context, deps Deps, pluginVersion string) error {
	fmt.Fprintln(deps.Stdout, "\n=== Refreshing plugin cache (nerv@nerv) ===")
	cli := claude.PluginCLI{Runner: deps.Runner}

	fmt.Fprintln(deps.Stdout, "-> claude plugin uninstall nerv@nerv")
	if _, err := cli.Uninstall(ctx, pluginID); err != nil {
		return fmt.Errorf("cache not refreshed: %w", err)
	}

	fmt.Fprintln(deps.Stdout, "-> claude plugin install nerv@nerv")
	if _, err := cli.Install(ctx, pluginID); err != nil {
		return fmt.Errorf("cache not refreshed: %w", err)
	}

	installedPluginsPath := filepath.Join(deps.Home, ".claude", "plugins", "installed_plugins.json")
	info, found, err := claude.InstalledPlugins(installedPluginsPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", installedPluginsPath, err)
	}
	if !found {
		return &RefusalError{Err: fmt.Errorf("nerv@nerv not found in %s after install", installedPluginsPath)}
	}
	if info.Version != pluginVersion {
		return &RefusalError{Err: fmt.Errorf(
			"cached plugin version %s does not match the embedded version %s; the plugin cache did not refresh as expected",
			info.Version, pluginVersion)}
	}

	fmt.Fprintf(deps.Stdout, "nerv@nerv version : %s (matches embedded; installPath: %s)\n", info.Version, info.InstallPath)
	return nil
}

func ensureEngram(ctx context.Context, deps Deps) {
	result := engram.EnsureKnowledgeBase(ctx, deps.Runner, deps.LookPath)
	for _, m := range result.Messages {
		fmt.Fprintln(deps.Stdout, m)
	}
	for _, w := range result.Warnings {
		fmt.Fprintf(deps.Stdout, "Warning: %s\n", w)
	}
}

func installSkills(ctx context.Context, deps Deps) {
	fmt.Fprintln(deps.Stdout, "\n=== Installing required skills ===")

	manifest, err := skills.LoadManifestFS(deps.FS)
	if err != nil {
		fmt.Fprintf(deps.Stdout, "Warning: could not read the skills manifest: %v\n", err)
		return
	}

	skillsDir := filepath.Join(deps.Home, ".claude", "skills")
	statuses := skills.Status(manifest, skillsDir)
	plan := skills.InstallPlan(statuses)

	for _, remedy := range plan.Remedies {
		fmt.Fprintln(deps.Stdout, remedy)
	}

	result := skills.Install(ctx, deps.Runner, plan)
	fmt.Fprintf(deps.Stdout, "skills: %d installed, %d failed\n", result.Installed, result.Failed)
	if result.Failed > 0 {
		fmt.Fprintln(deps.Stdout, "Warning: install-skills reported failures; see the lines above.")
	}
}
