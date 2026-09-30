package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/war-apps/nerv-gentle-ai/internal/claude"
)

// Uninstall removes NERV's settings.json registration (backed up and
// written atomically, like Install's own registration step), tolerantly
// runs "claude plugin uninstall nerv@nerv", and removes the materialized
// <home>/.nerv/marketplace directory. Mirrors install.ps1's -Uninstall
// branch plus the marketplace directory this package's embedded-plugin
// model introduces (the old script had nothing to remove there, since it
// registered the repo checkout itself).
func Uninstall(ctx context.Context, deps Deps) error {
	path := settingsPath(deps)
	previous, settings, err := loadOrInitSettings(path)
	if err != nil {
		return err
	}

	if settings.Unregister(marketplaceName, pluginID) {
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
	fmt.Fprintln(deps.Stdout, "-> claude plugin uninstall nerv@nerv")
	if _, err := cli.Uninstall(ctx, pluginID); err != nil {
		return fmt.Errorf("claude plugin uninstall failed: %w", err)
	}

	marketplaceDir := filepath.Join(deps.Home, ".nerv", "marketplace")
	if _, statErr := os.Stat(marketplaceDir); statErr == nil {
		if err := os.RemoveAll(marketplaceDir); err != nil {
			return fmt.Errorf("removing %s: %w", marketplaceDir, err)
		}
		fmt.Fprintf(deps.Stdout, "Removed %s\n", marketplaceDir)
	}

	fmt.Fprintln(deps.Stdout, "\nRestart Claude Code for the change to take effect.")
	return nil
}
