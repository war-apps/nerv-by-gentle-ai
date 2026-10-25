package install

import (
	"context"
	"fmt"
	"os"

	"github.com/war-apps/nerv-by-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

// Uninstall tolerantly runs "claude plugin uninstall" for every enabled
// extension and provider of the nerv marketplace, then removes NERV's
// settings.json registration (backed up and written atomically, like
// Install's own registration step) including every other "*@nerv" id,
// runs "claude plugin uninstall nerv@nerv" and "claude plugin marketplace
// remove nerv", and removes the materialized <home>/.nerv/marketplace
// directory. Extensions are uninstalled before settings.json changes, so
// a failed one is still enabled, and retried, on the next run.
func Uninstall(ctx context.Context, deps Deps) error {
	path := settingsPath(deps)
	previous, settings, err := loadOrInitSettings(path)
	if err != nil {
		return err
	}

	var extIDs []string
	for _, id := range settings.MarketplacePlugins(marketplaceName) {
		if id != pluginID {
			extIDs = append(extIDs, id)
		}
	}

	cli := claude.PluginCLI{Runner: deps.Runner}
	for _, id := range extIDs {
		if !settings.PluginEnabled(id) {
			continue
		}
		fmt.Fprintf(deps.Stdout, "-> claude plugin uninstall %s\n", id)
		if _, err := cli.Uninstall(ctx, id); err != nil {
			return fmt.Errorf("claude plugin uninstall failed (re-run nerv uninstall to finish): %w", err)
		}
	}

	removed := false
	for _, id := range extIDs {
		if settings.Unregister(marketplaceName, id) {
			removed = true
		}
	}
	if settings.Unregister(marketplaceName, pluginID) || removed {
		written, backup, err := claude.SaveSettings(path, settings, previous, deps.Now())
		if err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		if written {
			if backup != "" {
				fmt.Fprintf(deps.Stdout, "Backup written: %s\n", backup)
			}
			fmt.Fprintln(deps.Stdout, "Removed extraKnownMarketplaces.nerv and enabledPlugins[\"nerv@nerv\"].")
			for _, id := range extIDs {
				fmt.Fprintf(deps.Stdout, "Removed enabledPlugins[%q].\n", id)
			}
		}
	} else {
		fmt.Fprintln(deps.Stdout, "extraKnownMarketplaces.nerv and enabledPlugins[\"nerv@nerv\"] not present.")
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
