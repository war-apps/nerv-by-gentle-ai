package claude

import (
	"context"
	"fmt"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/env"
)

// PluginCLI drives Claude Code's "claude plugin" subcommand — the
// marketplace add plus the uninstall/install pair that refreshes the plugin
// cache — through an injected env.Runner.
type PluginCLI struct {
	Runner env.Runner
}

// AddMarketplace runs "claude plugin marketplace add <source>", syncing the
// marketplace registry from the materialized directory so the following
// install resolves the current plugin. Any non-zero exit, or a launch
// failure, is returned as an error carrying the CLI output.
func (c PluginCLI) AddMarketplace(ctx context.Context, source string) (output string, err error) {
	stdout, stderr, exitCode, err := c.Runner.Run(ctx, "claude", "plugin", "marketplace", "add", source)
	if err != nil {
		return "", fmt.Errorf("claude plugin marketplace add %s could not run: %w", source, err)
	}
	combined := stdout + stderr
	if exitCode != 0 {
		return combined, fmt.Errorf("claude plugin marketplace add %s exited with code %d: %s", source, exitCode, strings.TrimSpace(combined))
	}
	return combined, nil
}

// Uninstall runs "claude plugin uninstall <pluginID>". A non-zero exit
// whose combined output mentions "not installed" (case-insensitively) is
// tolerated and reported as success, matching the fact that uninstalling
// an already-absent plugin is not a failure for our purposes. Any other
// non-zero exit, or a launch failure, is returned as an error.
func (c PluginCLI) Uninstall(ctx context.Context, pluginID string) (output string, err error) {
	return c.run(ctx, "uninstall", pluginID, true)
}

// Install runs "claude plugin install <pluginID>". Any non-zero exit, or
// a launch failure, is returned as an error.
func (c PluginCLI) Install(ctx context.Context, pluginID string) (output string, err error) {
	return c.run(ctx, "install", pluginID, false)
}

func (c PluginCLI) run(ctx context.Context, verb, pluginID string, tolerateNotInstalled bool) (string, error) {
	stdout, stderr, exitCode, err := c.Runner.Run(ctx, "claude", "plugin", verb, pluginID)
	if err != nil {
		return "", fmt.Errorf("claude plugin %s %s could not run: %w", verb, pluginID, err)
	}
	combined := stdout + stderr
	if exitCode != 0 {
		if tolerateNotInstalled && strings.Contains(strings.ToLower(combined), "not installed") {
			return combined, nil
		}
		return combined, fmt.Errorf("claude plugin %s %s exited with code %d: %s", verb, pluginID, exitCode, strings.TrimSpace(combined))
	}
	return combined, nil
}
