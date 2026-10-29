package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-by-gentle-ai/internal/engram"
	"github.com/war-apps/nerv-by-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
	"github.com/war-apps/nerv-by-gentle-ai/internal/skills"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
)

const (
	// PluginID is the NERV plugin's id in Claude Code.
	PluginID = "nerv@nerv"
	// MarketplaceName is the marketplace NERV and its extensions install
	// from.
	MarketplaceName = "nerv"
)

// Install runs the whole "nerv install" use case: the gentle-ai preflight,
// materializing the embedded plugin under <home>/.nerv/marketplace,
// registering that directory marketplace and enabling nerv@nerv in
// settings.json, refreshing the plugin cache through "claude plugin
// uninstall/install" and verifying it against the embedded version,
// reporting (and, on confirmation, removing) the old /task:* command copies
// that shadow the task plugin's commands, applying model/effort assignments to the cached agents, ensuring the
// Engram "nerv" knowledge base, and installing skills (unless
// opts.NoSkills). The closing "run nerv configure" hint is cmd/nerv's own
// responsibility, not this package's. Every step's progress is written to
// deps.Stdout as it runs.
func Install(ctx context.Context, deps Deps, opts Options) error {
	if err := preflightGentleAI(ctx, deps, opts.RequireGentleAI); err != nil {
		return err
	}

	marketplaceDir := paths.Resolve(deps.Home).Marketplace
	matResult, err := plugin.MaterializeBundle(deps.FS, deps.ExtensionsFS, marketplaceDir)
	if err != nil {
		return fmt.Errorf("materializing plugin: %w", err)
	}
	fmt.Fprintf(deps.Stdout, "Plugin materialized at %s (%d written, %d unchanged, %d removed)\n",
		marketplaceDir, len(matResult.Written), len(matResult.Unchanged), len(matResult.Removed))

	selected, retired, err := registerSettings(deps, opts, marketplaceDir)
	if err != nil {
		return err
	}
	uninstallRetired(ctx, deps, retired)

	pluginVersion, err := version.PluginVersion(deps.FS)
	if err != nil {
		return fmt.Errorf("reading embedded plugin version: %w", err)
	}
	if err := refreshCache(ctx, deps, pluginVersion); err != nil {
		return err
	}
	for _, ext := range selected {
		if err := refreshPlugin(ctx, deps, ext.Name+"@"+MarketplaceName, ext.Version); err != nil {
			return err
		}
	}

	reportLegacyCommands(deps)

	if err := applyModelsForVersion(deps, pluginVersion); err != nil {
		return err
	}

	ensureEngram(ctx, deps)

	if !opts.NoSkills {
		installSkills(ctx, deps)
	}

	fmt.Fprintln(deps.Stdout, "\nRestart Claude Code for the change to take effect.")
	return nil
}

// upgradeHint is printed for a 3.x gentle-ai. `gentle-ai upgrade` from 3.x
// cannot reach 4.x on go installs, so it is not suggested.
const upgradeHint = "  Upgrade: go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@latest (or: brew upgrade gentle-ai), then: gentle-ai sync.\n" +
	"  Note: `gentle-ai upgrade` from 3.x cannot reach 4.x on go installs."

func preflightGentleAI(ctx context.Context, deps Deps, require bool) error {
	preflight := gentleai.CheckPreflight(ctx, deps.Runner)

	switch {
	case !preflight.Found:
		fmt.Fprintln(deps.Stdout, "Warning: gentle-ai not found on PATH; NERV requires gentle-ai 4.x (https://github.com/Gentleman-Programming/gentle-ai)")
		if require {
			return &RefusalError{Err: errors.New("gentle-ai not found on PATH")}
		}
	case preflight.Version == "":
		fmt.Fprintln(deps.Stdout, "Warning: gentle-ai --version returned an unparseable value")
		if require {
			return &RefusalError{Err: errors.New("gentle-ai --version returned an unparseable value")}
		}
	case !preflight.OK:
		fmt.Fprintf(deps.Stdout, "Warning: NERV requires gentle-ai 4.x; found %s\n", preflight.Version)
		if strings.HasPrefix(preflight.Version, "3.") {
			fmt.Fprintln(deps.Stdout, upgradeHint)
		}
		if require {
			return &RefusalError{Err: fmt.Errorf("NERV requires gentle-ai 4.x; found %s", preflight.Version)}
		}
	default:
		fmt.Fprintf(deps.Stdout, "gentle-ai version : %s (tested against 4.3.1)\n", preflight.Version)
	}
	return nil
}

func settingsPath(deps Deps) string {
	if deps.SettingsPath != "" {
		return deps.SettingsPath
	}
	return paths.Resolve(deps.Home).Settings
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

// registerSettings registers the marketplace and enables nerv@nerv plus
// the extensions selectExtensions picks, returning those extensions.
func registerSettings(deps Deps, opts Options, marketplaceDir string) (selected []plugin.Extension, retired []string, err error) {
	path := settingsPath(deps)
	previous, settings, err := loadOrInitSettings(path)
	if err != nil {
		return nil, nil, err
	}

	selected, err = selectExtensions(deps, opts, settings)
	if err != nil {
		return nil, nil, err
	}

	changed := settings.RegisterMarketplace(MarketplaceName, marketplaceDir)
	for _, name := range RetiredPlugins {
		if id := name + "@" + MarketplaceName; settings.RemovePlugin(id) {
			retired = append(retired, id)
			changed = true
		}
	}
	if settings.EnablePlugin(PluginID) {
		changed = true
	}
	for _, ext := range selected {
		if settings.EnablePlugin(ext.Name + "@" + MarketplaceName) {
			changed = true
		}
	}

	if !changed {
		fmt.Fprintln(deps.Stdout, "settings.json already up to date.")
		return selected, nil, nil
	}

	written, backup, err := claude.SaveSettings(path, settings, previous, deps.Now())
	if err != nil {
		return nil, nil, fmt.Errorf("writing %s: %w", path, err)
	}
	if written {
		if backup != "" {
			fmt.Fprintf(deps.Stdout, "Backup written: %s\n", backup)
		}
		fmt.Fprintln(deps.Stdout, "settings.json updated and verified.")
	}
	for _, id := range retired {
		fmt.Fprintf(deps.Stdout, "Removed %s from settings.json (%s)\n", id, retiredWhy)
	}
	return selected, retired, nil
}

// RetiredPlugins are the plugins of the nerv marketplace this nerv no
// longer ships: the deploy extension and its providers, merged into
// release (release-github, release-ssh-compose, release-script). Install
// removes them from enabledPlugins and from Claude Code's plugin cache.
var RetiredPlugins = []string{"deploy", "deploy-github", "deploy-ssh-compose", "deploy-bash"}

// retiredWhy says where a retired plugin went.
const retiredWhy = "merged into the release extension"

// uninstallRetired runs "claude plugin uninstall" for each retired plugin
// id, tolerantly: settings.json no longer enables it, so a failure only
// leaves a stale cache entry, reported.
func uninstallRetired(ctx context.Context, deps Deps, ids []string) {
	cli := claude.PluginCLI{Runner: deps.Runner}
	for _, id := range ids {
		fmt.Fprintf(deps.Stdout, "-> claude plugin uninstall %s\n", id)
		if _, err := cli.Uninstall(ctx, id); err != nil {
			fmt.Fprintf(deps.Stdout, "Warning: claude plugin uninstall %s failed (%v); remove it with /plugin in Claude Code\n", id, err)
		}
	}
}

// RefreshCache re-registers nerv@nerv in the Claude Code plugin cache
// ("claude plugin marketplace add", then "uninstall/install") and verifies
// the cached version matches the embedded plugin version, then does the
// same for every extension and provider of deps.ExtensionsFS that settings.json
// enables, at that extension's embedded version. Exported for
// internal/wizard's closing "apply and refresh" step, wrapping the same tail
// Install itself runs after registering settings.json.
func RefreshCache(ctx context.Context, deps Deps) error {
	pluginVersion, err := version.PluginVersion(deps.FS)
	if err != nil {
		return fmt.Errorf("reading embedded plugin version: %w", err)
	}
	if err := refreshCache(ctx, deps, pluginVersion); err != nil {
		return err
	}

	exts, err := plugin.Extensions(deps.ExtensionsFS)
	if err != nil || len(exts) == 0 {
		return err
	}
	_, settings, err := loadOrInitSettings(settingsPath(deps))
	if err != nil {
		return err
	}
	for _, ext := range exts {
		id := ext.Name + "@" + MarketplaceName
		if !settings.PluginEnabled(id) {
			continue
		}
		if err := refreshPlugin(ctx, deps, id, ext.Version); err != nil {
			return err
		}
	}
	return nil
}

func refreshCache(ctx context.Context, deps Deps, pluginVersion string) error {
	fmt.Fprintln(deps.Stdout, "\n=== Refreshing plugin cache (nerv@nerv) ===")
	cli := claude.PluginCLI{Runner: deps.Runner}

	marketplaceDir := paths.Resolve(deps.Home).Marketplace
	fmt.Fprintf(deps.Stdout, "-> claude plugin marketplace add %s\n", marketplaceDir)
	if _, err := cli.AddMarketplace(ctx, marketplaceDir); err != nil {
		return fmt.Errorf("cache not refreshed: %w", err)
	}
	return refreshPlugin(ctx, deps, PluginID, pluginVersion)
}

// refreshPlugin reinstalls id from the already registered marketplace and
// verifies the cached version is wantVersion.
func refreshPlugin(ctx context.Context, deps Deps, id, wantVersion string) error {
	cli := claude.PluginCLI{Runner: deps.Runner}
	pluginVersion := wantVersion

	fmt.Fprintf(deps.Stdout, "-> claude plugin uninstall %s\n", id)
	if _, err := cli.Uninstall(ctx, id); err != nil {
		return fmt.Errorf("cache not refreshed: %w", err)
	}

	fmt.Fprintf(deps.Stdout, "-> claude plugin install %s\n", id)
	if _, err := cli.Install(ctx, id); err != nil {
		return fmt.Errorf("cache not refreshed: %w", err)
	}

	installedPluginsPath := paths.Resolve(deps.Home).InstalledPlugins
	info, found, err := claude.InstalledPlugin(installedPluginsPath, id)
	if err != nil {
		return fmt.Errorf("reading %s: %w", installedPluginsPath, err)
	}
	if !found {
		return &RefusalError{Err: fmt.Errorf("%s not found in %s after install", id, installedPluginsPath)}
	}
	if info.Version != pluginVersion {
		return &RefusalError{Err: fmt.Errorf(
			"cached plugin version %s does not match the embedded version %s; the plugin cache did not refresh as expected",
			info.Version, pluginVersion)}
	}

	fmt.Fprintf(deps.Stdout, "%s version : %s (matches embedded; installPath: %s)\n", displayName(id), info.Version, info.InstallPath)
	return nil
}

// displayName is how output names a plugin: an extension or provider by
// its bare name ("task"), the core plugin by its id (nerv@nerv). The
// "-> claude plugin ..." lines echo the real command, so they keep the id.
func displayName(id string) string {
	if id == PluginID {
		return id
	}
	return strings.TrimSuffix(id, "@"+MarketplaceName)
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

	skillsDir := paths.Resolve(deps.Home).SkillsDir
	report, err := skills.Run(ctx, deps.Runner, deps.FS, skillsDir, skills.Options{ConfirmSync: deps.ConfirmSync})
	if err != nil {
		fmt.Fprintf(deps.Stdout, "Warning: could not read the skills manifest: %v\n", err)
		return
	}

	skills.RenderSync(deps.Stdout, report)
	for _, remedy := range report.Plan.Remedies {
		fmt.Fprintln(deps.Stdout, remedy)
	}

	fmt.Fprintf(deps.Stdout, "skills: %d installed, %d failed\n", report.Result.Installed, report.Result.Failed)
	if report.Result.Failed > 0 {
		fmt.Fprintln(deps.Stdout, "Warning: install-skills reported failures; see the lines above.")
	}
}
