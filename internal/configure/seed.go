package configure

import (
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
	"gopkg.in/yaml.v3"
)

// SeedSuffix ends the To of a change InitRepo copied from the user's
// defaults.tasks.
const SeedSuffix = " (from defaults.tasks)"

// seedTasks writes the user's defaults.tasks (workflowconfig.Seed: the
// personal keys left out) as doc's tasks: section when doc has none, and
// returns one change per value it copied. userConfig is the user file
// (empty: the one of deps.Home); no user file or no seed copies nothing.
func seedTasks(deps Deps, userConfig string, doc *config.Document) ([]Change, error) {
	if userConfig == "" && deps.Home != "" {
		userConfig = paths.Resolve(deps.Home).UserConfig
	}
	if userConfig == "" || doc.KeyExists("tasks") {
		return nil, nil
	}
	user, exists, err := (Store{}).Load(userConfig)
	if err != nil || !exists {
		return nil, err
	}
	personal, err := workflowconfig.LoadPersonal(deps.ExtensionsFS)
	if err != nil {
		return nil, err
	}
	block, ok := workflowconfig.Seed(user, "tasks", personal)
	if !ok {
		return nil, nil
	}
	doc.SetBlock("tasks", block)
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(block), &root); err != nil || len(root.Content) == 0 {
		return nil, err
	}
	var changes []Change
	leaves(root.Content[0], "", func(path, value string) {
		changes = append(changes, Change{Key: path, To: value + SeedSuffix})
	})
	return changes, nil
}

// leaves calls f with the dotted path and value of each scalar under n,
// in document order; a list is one value, "[...]".
func leaves(n *yaml.Node, path string, f func(path, value string)) {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			if path != "" {
				key = path + "." + key
			}
			leaves(n.Content[i+1], key, f)
		}
	case yaml.SequenceNode:
		f(path, "[...]")
	case yaml.ScalarNode:
		f(path, n.Value)
	}
}
