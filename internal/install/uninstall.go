package install

import (
	"context"
	"fmt"
	"os"

	"github.com/war-apps/nerv-by-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

// Uninstall removes NERV's settings.json registration (backed up and
// written atomically, like Install's own registration step), tolerantly
// runs "claude plugin uninstall" for every enabled extension plugin and
// then nerv@nerv, runs "claude plugin marketplace remove nerv", and
// removes the materialized <home>/.nerv/marketplace directory.
func Uninstall(ctx context.Context, deps Deps) error {
	path := settingsPath(deps)
	previous, settings, err := loadOrInitSettings(path)
	if err != nil {
		return err
	}

	exts, err := plugin.Extensions(deps.ExtensionsFS)
	if err != nil {
		return err
	}
	var extIDs []string
	for _, e := range exts {
		if id := e.Name + "@" + marketplaceName; settings.PluginEnabled(id) {
			extIDs = append(extIDs, id)
		}
	}

	removedExt := false
	for _, id := range extIDs {
		if settings.Unregister(marketplaceName, id) {
			removedExt = true
		}
	}
	if settings.Unregister(marketplaceName, pluginID) || removedExt {
		written, backup, err := claude.SaveSettings(path, settings, previous, deps.Now())
		if err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		if written {
			if backup != "" {
				fmt.Fprintf(deps.Stdout, "Backup written: %s\n", backup)
			}
			fmt.Fprintln(deps.Stdout, "Removed extraKnownMarketplaces.nerv and enabledPlugins[\"nerv@nerv\"].")
		}
	} else {
		fmt.Fprintln(deps.Stdout, "extraKnownMarketplaces.nerv and enabledPlugins[\"nerv@nerv\"] not present.")
	}

	cli := claude.PluginCLI{Runner: deps.Runner}
	for _, id := range extIDs {
		fmt.Fprintf(deps.Stdout, "-> claude plugin uninstall %s\n", id)
		if _, err := cli.Uninstall(ctx, id); err != nil {
			return fmt.Errorf("claude plugin uninstall failed: %w", err)
		}
	}
	fmt.Fprintln(deps.Stdout, "-> claude plugin uninstall nerv@nerv")
	if _, err := cli.Uninstall(ctx, pluginID); err != nil {
		return fmt.Errorf("claude plugin uninstall failed: %w", err)
	}

	fmt.Fprintf(deps.Stdout, "-> claude plugin marketplace remove %s\n", marketplaceName)
	if _, err := cli.RemoveMarketplace(ctx, marketplaceName); err != nil {
		return fmt.Errorf("claude plugin marketplace remove failed (re-run nerv uninstall to finish): %w", err)
	}

	marketplaceDir := paths.Resolve(deps.Home).Marketplace
	if _, statErr := os.Stat(marketplaceDir); statErr == nil {
		if err := os.RemoveAll(marketplaceDir); err != nil {
			return fmt.Errorf("removing %s: %w", marketplaceDir, err)
		}
		fmt.Fprintf(deps.Stdout, "Removed %s\n", marketplaceDir)
	}

	fmt.Fprintln(deps.Stdout, "\nRestart Claude Code for the change to take effect.")
	return nil
}
