package configure

import (
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
	"github.com/war-apps/nerv-by-gentle-ai/internal/taskprovider"
)

// Provider is a task provider plugin shipped in the extensions tree, with
// its parsed provider.yaml. Core knows providers only through manifests:
// no provider name or key is written into this package.
type Provider struct {
	// Plugin is the extension plugin's name, such as "task-<name>".
	Plugin   string
	Manifest *taskprovider.Manifest
}

// Key returns the manifest's config key named key.
func (p Provider) Key(key string) (taskprovider.ConfigKey, bool) {
	i := slices.IndexFunc(p.Manifest.Config, func(c taskprovider.ConfigKey) bool { return c.Key == key })
	if i < 0 {
		return taskprovider.ConfigKey{}, false
	}
	return p.Manifest.Config[i], true
}

// EmbeddedProviders loads the provider.yaml of every task provider plugin
// in fsys (typically nerv.ExtensionsFS()), in plugin name order. A nil
// fsys has none.
func EmbeddedProviders(fsys fs.FS) ([]Provider, error) {
	exts, err := plugin.Extensions(fsys)
	if err != nil {
		return nil, err
	}
	var providers []Provider
	for _, ext := range exts {
		if ext.Owner != plugin.TaskPlugin {
			continue
		}
		m, err := taskprovider.Load(fsys, ext.Name)
		if err != nil {
			return nil, err
		}
		providers = append(providers, Provider{Plugin: ext.Name, Manifest: m})
	}
	return providers, nil
}

// ProviderKeyPath is where the configuration holds a provider's config key.
func ProviderKeyPath(provider, key string) string {
	return "tasks.providers." + provider + "." + key
}

// ValidateProviderValue reports whether value suits c's type: an integer
// for int, true or false for bool, anything for string. A map or list key
// takes no scalar value.
func ValidateProviderValue(provider string, c taskprovider.ConfigKey, value string) error {
	valid := true
	switch c.Type {
	case "int":
		_, err := strconv.Atoi(value)
		valid = err == nil
	case "bool":
		_, err := strconv.ParseBool(value)
		valid = err == nil
	case "string":
	default:
		valid = false
	}
	if !valid {
		return fmt.Errorf("Invalid value '%s' for %s: expected %s.", value, ProviderKeyPath(provider, c.Key), c.Type)
	}
	return nil
}

// ProviderSetting is one configuration key a provider owns (Key is the full
// dotted path, such as tasks.providers.<name>.<key>) and its new value.
type ProviderSetting struct {
	Key   string
	Value string
}

// ParseProviderConfig parses and validates --repo-provider-config entries,
// each "<provider>.<key>=<value>", against the manifests embedded in fsys.
// The first bad entry refuses them all.
func ParseProviderConfig(fsys fs.FS, args []string) ([]ProviderSetting, error) {
	if len(args) == 0 {
		return nil, nil
	}
	providers, err := EmbeddedProviders(fsys)
	if err != nil {
		return nil, err
	}

	settings := make([]ProviderSetting, 0, len(args))
	for _, raw := range args {
		dotted, value, ok := config.ParseSetArg(raw)
		name, key, hasKey := strings.Cut(dotted, ".")
		if !ok || !hasKey || name == "" || key == "" {
			return nil, &RefusalError{Err: fmt.Errorf("Invalid --repo-provider-config entry '%s'; expected <provider>.<key>=<value>.", raw)}
		}

		i := slices.IndexFunc(providers, func(p Provider) bool { return p.Manifest.Name == name })
		if i < 0 {
			available := "none"
			if len(providers) > 0 {
				names := make([]string, len(providers))
				for j, p := range providers {
					names[j] = p.Manifest.Name
				}
				available = strings.Join(names, ", ")
			}
			return nil, &RefusalError{Err: fmt.Errorf("Unknown task provider '%s' in --repo-provider-config: no embedded manifest (available: %s).", name, available)}
		}

		c, found := providers[i].Key(key)
		if !found {
			keys := make([]string, len(providers[i].Manifest.Config))
			for j, k := range providers[i].Manifest.Config {
				keys[j] = k.Key
			}
			return nil, &RefusalError{Err: fmt.Errorf("Unknown key '%s' for task provider '%s'. Keys: %s.", key, name, strings.Join(keys, ", "))}
		}
		if err := ValidateProviderValue(name, c, value); err != nil {
			return nil, &RefusalError{Err: err}
		}
		if c.Scope != "repo" {
			return nil, &RefusalError{Err: fmt.Errorf("Key '%s' of task provider '%s' is %s-scope; set it with nerv configure, not --repo-provider-config.", key, name, c.Scope)}
		}
		settings = append(settings, ProviderSetting{Key: ProviderKeyPath(name, key), Value: value})
	}
	return settings, nil
}

// applyProviderSettings writes each setting into doc with the
// format-preserving editor, returning what changed. A value equal to the
// key's current one is skipped.
func applyProviderSettings(doc *config.Document, settings []ProviderSetting) []Change {
	changes := emptyChanges()
	for _, s := range settings {
		current, _ := doc.Scalar(s.Key)
		if current == s.Value {
			continue
		}
		doc.SetScalar(s.Key, s.Value)
		changes = append(changes, Change{Key: s.Key, From: current, To: s.Value})
	}
	return changes
}

// SetProviderSettings writes provider-owned keys (validated by the caller
// against the provider manifests) into the configuration file at path, through the
// same Store and backup rule as Set. Every other byte of the file is kept.
func SetProviderSettings(deps Deps, path string, settings []ProviderSetting) (Result, error) {
	store := Store{}
	doc, exists, err := store.Load(path)
	if err != nil {
		return Result{}, err
	}
	original := []byte(doc.String())

	working := doc
	if !exists {
		working = config.Parse(missingUserConfigHeader)
	}
	changes := applyProviderSettings(working, settings)
	return finalizeMutation(deps, Paths{Config: path}, store, working, original, len(changes) > 0, changes)
}
