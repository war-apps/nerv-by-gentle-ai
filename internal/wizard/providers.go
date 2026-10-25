package wizard

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"

	"github.com/war-apps/nerv-by-gentle-ai/internal/claude"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
	"github.com/war-apps/nerv-by-gentle-ai/internal/taskprovider"
)

// marketplace is the marketplace every nerv extension and provider
// installs from.
const marketplace = "nerv"

// taskPlugins reports whether settings.json enables the task extension
// and, when it does, the embedded providers it also enables, in plugin name
// order. Core names no provider: they come from the manifests in
// deps.ExtensionsFS. A missing settings.json enables nothing; an unreadable
// one is reported and treated the same way.
func taskPlugins(deps Deps, opts Options, out io.Writer) (enabled bool, providers []configure.Provider) {
	settingsPath := opts.SettingsPath
	if settingsPath == "" {
		settingsPath = paths.Resolve(deps.Home).Settings
	}
	settings, err := claude.LoadSettings(settingsPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(out, "Warning: could not read %s (%v); skipping the task provider settings.\n", settingsPath, err)
		}
		return false, nil
	}
	if !settings.PluginEnabled(plugin.TaskPlugin + "@" + marketplace) {
		return false, nil
	}

	all, err := configure.EmbeddedProviders(deps.ExtensionsFS)
	if err != nil {
		fmt.Fprintf(out, "Warning: could not read the provider manifests (%v); skipping the task provider settings.\n", err)
		return true, nil
	}
	for _, p := range all {
		if settings.PluginEnabled(p.Plugin + "@" + marketplace) {
			providers = append(providers, p)
		}
	}
	return true, providers
}

// providerNames lists the providers' names, in order.
func providerNames(providers []configure.Provider) []string {
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Manifest.Name
	}
	return names
}

// askProviderKey prompts one scalar provider key with its manifest Prompt,
// pre-filled with current: a bool is a true|false choice, an int is asked
// again (up to maxInvalidAttempts times, then current is kept) until the
// answer is an integer, a string takes any answer.
func askProviderKey(s *session, provider string, c taskprovider.ConfigKey, current string) (string, error) {
	if c.Type == "bool" {
		return s.choose(c.Prompt, []string{"true", "false"}, current)
	}
	for attempt := 0; attempt < maxInvalidAttempts; attempt++ {
		answer, err := s.ask(c.Prompt, current)
		if err != nil {
			return "", err
		}
		if answer == "" || configure.ValidateProviderValue(provider, c, answer) == nil {
			return answer, nil
		}
		fmt.Fprintf(s.out, "Invalid input. Expected %s.\n", c.Type)
	}
	return current, nil
}

// scalarKeys are a manifest's prompted keys of one scope, in manifest order.
func scalarKeys(m *taskprovider.Manifest, scope string) []taskprovider.ConfigKey {
	var keys []taskprovider.ConfigKey
	for _, c := range m.Config {
		if c.Scope == scope && c.Scalar() {
			keys = append(keys, c)
		}
	}
	return keys
}

// runTaskProvidersSection is the Task providers section, shown only when
// the task extension is enabled: tasks.provider when more than one provider is
// enabled, then every user-scope scalar key of each enabled provider's
// manifest, shown with its current value or its Default. The changed
// answers are written to the user config.yaml in one
// configure.SetProviderSettings batch, after a summary and a confirmation.
func runTaskProvidersSection(deps Deps, cfg configure.Paths, providers []configure.Provider, s *session, out io.Writer) (bool, error) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- Task providers ---")
	if len(providers) == 0 {
		fmt.Fprintln(out, "No task provider is enabled; install one with nerv install.")
		return false, nil
	}

	doc, _, err := (configure.Store{}).Load(cfg.Config)
	if err != nil {
		return false, err
	}
	current := func(key, fallback string) string {
		if v, found := doc.Scalar(key); found {
			return v
		}
		return fallback
	}

	var settings []configure.ProviderSetting
	var changeLog []string
	record := func(key, oldValue, newValue string) {
		if newValue != oldValue {
			settings = append(settings, configure.ProviderSetting{Key: key, Value: newValue})
			changeLog = append(changeLog, fmt.Sprintf("%s: %s -> %s", key, oldValue, newValue))
		}
	}

	if names := providerNames(providers); len(names) > 1 {
		old := current("tasks.provider", "")
		def := old
		if !slices.Contains(names, def) {
			def = names[0]
		}
		answer, err := s.choose("Task provider", names, def)
		if err != nil {
			return false, err
		}
		record("tasks.provider", old, answer)
	}

	for _, p := range providers {
		name := p.Manifest.Name
		fmt.Fprintln(out)
		fmt.Fprintf(out, "-- %s --\n", name)
		for _, c := range scalarKeys(p.Manifest, "user") {
			key := configure.ProviderKeyPath(name, c.Key)
			old := current(key, c.Default)
			answer, err := askProviderKey(s, name, c, old)
			if err != nil {
				return false, err
			}
			record(key, old, answer)
		}
	}

	fmt.Fprintln(out)
	if len(settings) == 0 {
		fmt.Fprintf(out, "No changes; %s left untouched.\n", cfg.Config)
		return false, nil
	}
	fmt.Fprintln(out, "--- Summary ---")
	for _, c := range changeLog {
		fmt.Fprintf(out, "  %s\n", c)
	}
	write, err := s.yesNo(fmt.Sprintf("Write to %s?", cfg.Config), true)
	if err != nil {
		return false, err
	}
	if !write {
		fmt.Fprintln(out, "Aborted; no task provider settings written.")
		return false, nil
	}

	result, err := configure.SetProviderSettings(deps, cfg.Config, settings)
	if err != nil {
		return false, err
	}
	if result.Backup != nil {
		fmt.Fprintf(out, "Backup written: %s\n", *result.Backup)
	}
	if result.Changed {
		fmt.Fprintf(out, "Written: %s\n", cfg.Config)
	}
	return result.Changed, nil
}

// askRepoProviderKeys prompts every repo-scope scalar key of each enabled
// provider for a repository being initialized, returning the answered ones
// that differ from the key's Default as configure.InitRepoRequest
// ProviderConfig entries.
func askRepoProviderKeys(s *session, providers []configure.Provider) ([]string, error) {
	var entries []string
	for _, p := range providers {
		name := p.Manifest.Name
		for _, c := range scalarKeys(p.Manifest, "repo") {
			answer, err := askProviderKey(s, name, c, c.Default)
			if err != nil {
				return nil, err
			}
			if answer != "" && answer != c.Default {
				entries = append(entries, name+"."+c.Key+"="+answer)
			}
		}
	}
	return entries, nil
}
