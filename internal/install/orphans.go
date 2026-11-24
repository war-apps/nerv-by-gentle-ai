package install

import (
	"context"
	"fmt"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

// OrphanFamily is a group of plugins of the nerv marketplace that nerv no
// longer ships (the extensions and providers it used to bundle), all
// replaced by one NERV-ecosystem tool. Once nerv's marketplace has no
// source for them they are orphans: Install and DisableOrphans disable
// them.
type OrphanFamily struct {
	// Tool is the NERV-ecosystem tool that replaces them, such as
	// "nerv-task".
	Tool string
	// Commands names the slash commands the family brought, such as
	// "/task:*".
	Commands string
	// Plugins are the plugins' bare names ("task"), in the order they are
	// reported.
	Plugins []string
	// ToolInstalled reports whether Tool is installed (or chosen in this
	// run): the orphans are then only noted, one line each; otherwise the
	// user is warned that Commands are gone until Tool is installed.
	ToolInstalled bool
}

// orphansFound is one family's plugins found enabled in settings.json or
// cached in installed_plugins.json, as ids, in family order.
type orphansFound struct {
	family OrphanFamily
	ids    []string
}

// removeOrphans removes every family plugin from settings (enabled or
// not) and returns, per family with any, the ids removed from settings or
// still recorded in installed_plugins.json, and whether settings changed.
func removeOrphans(deps Deps, settings *claude.Settings, families []OrphanFamily) (found []orphansFound, changed bool) {
	installedPlugins := paths.Resolve(deps.Home).InstalledPlugins
	for _, f := range families {
		var ids []string
		for _, name := range f.Plugins {
			id := name + "@" + MarketplaceName
			removed := settings.RemovePlugin(id)
			_, cached, _ := claude.InstalledPlugin(installedPlugins, id)
			if removed {
				changed = true
			}
			if removed || cached {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			found = append(found, orphansFound{family: f, ids: ids})
		}
	}
	return found, changed
}

// reportOrphans says what happened to each family's orphans: a note per
// plugin when its tool is installed, otherwise one warning naming them,
// the commands lost and how to install the tool.
func reportOrphans(deps Deps, found []orphansFound) {
	for _, o := range found {
		if o.family.ToolInstalled {
			for _, id := range o.ids {
				fmt.Fprintf(deps.Stdout, "Removed %s from settings.json (replaced by %s)\n", id, o.family.Tool)
			}
			continue
		}
		fmt.Fprintf(deps.Stdout, "Warning: disabled %s: nerv no longer ships them, so %s is unavailable until you install %s: nerv install --tools %s\n",
			strings.Join(o.ids, ", "), o.family.Commands, o.family.Tool, o.family.Tool)
	}
}

// uninstallOrphans runs "claude plugin uninstall" for each orphan,
// tolerantly: settings.json no longer enables it, so a failure only
// leaves a stale cache entry, reported.
func uninstallOrphans(ctx context.Context, deps Deps, found []orphansFound) {
	cli := claude.PluginCLI{Runner: deps.Runner}
	for _, o := range found {
		for _, id := range o.ids {
			fmt.Fprintf(deps.Stdout, "-> claude plugin uninstall %s\n", id)
			if _, err := cli.Uninstall(ctx, id); err != nil {
				fmt.Fprintf(deps.Stdout, "Warning: claude plugin uninstall %s failed (%v); remove it with /plugin in Claude Code\n", id, err)
			}
		}
	}
}

// DisableOrphans disables the orphans of families on their own, as
// Install does while it registers nerv@nerv: removed from settings.json
// (backed up and written like Install's registration), reported, then
// uninstalled from Claude Code's plugin cache. nerv upgrade runs it after
// its ecosystem-tools step, once it knows which tools are installed.
func DisableOrphans(ctx context.Context, deps Deps, families []OrphanFamily) error {
	path := settingsPath(deps)
	previous, settings, err := loadOrInitSettings(path)
	if err != nil {
		return err
	}
	found, changed := removeOrphans(deps, settings, families)
	if len(found) == 0 {
		return nil
	}
	fmt.Fprintln(deps.Stdout)
	if changed {
		written, backup, err := claude.SaveSettings(path, settings, previous, deps.Now())
		if err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		if written && backup != "" {
			fmt.Fprintf(deps.Stdout, "Backup written: %s\n", backup)
		}
	}
	reportOrphans(deps, found)
	uninstallOrphans(ctx, deps, found)
	return nil
}
