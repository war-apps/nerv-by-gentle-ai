package releaseconfig

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// EngineProvider is the version engine whose keys the section held before
// the version slot (branches, release_rules, ...), and the slot's default:
// they now live under release.providers.<EngineProvider>.
const EngineProvider = "conventional-commits"

// MovedEngineKeys are the version engine keys the section held itself
// before they moved under release.providers.conventional-commits.
var MovedEngineKeys = []string{"branches", "release_rules", "note_keywords", "tag_format", "repository_url", "changelog"}

// The build providers the keys of a project's build: mapping moved to.
const (
	dockerComposeProvider = "docker-compose"
	bashProvider          = "bash"
)

// buildModes are the build modes a project's build: mapping named before
// build became the build provider's name, with that provider.
var buildModes = map[string]string{"compose": dockerComposeProvider, "script": bashProvider}

// buildMove is where one key of a project's build: mapping went: under
// the project's providers.<provider> (provider set), or to the project
// itself (provider empty; mode became build itself).
type buildMove struct {
	key, provider string
	// message names the new place (%s is the project's path,
	// release.projects[i]).
	message string
}

// movedBuildKeys are the keys of the build: mapping a project held before
// build became the build provider's name, with their new place. MoveKeys
// moves them there, and Load names it.
var movedBuildKeys = []buildMove{
	{"mode", "", "build is the build provider's name now: %[1]s.build: docker-compose (was mode compose) or %[1]s.build: bash (was mode script)"},
	{"file", dockerComposeProvider, "moved to %s.providers.docker-compose.file (or release.providers.docker-compose.file for every project)"},
	{"test_service", dockerComposeProvider, "moved to %s.providers.docker-compose.test_service (or release.providers.docker-compose.test_service for every project)"},
	{"shared_host", bashProvider, "moved to %s.providers.bash.shared_host (or release.providers.bash.shared_host for every project)"},
	{"images", "", "moved to %s.images"},
	{"version_files", "", "moved to %s.version_files"},
	{"coverage", "", "moved to %s.coverage"},
}

// movedBuildKey is the move of a build: mapping key.
func movedBuildKey(key string) (buildMove, bool) {
	i := slices.IndexFunc(movedBuildKeys, func(m buildMove) bool { return m.key == key })
	if i < 0 {
		return buildMove{}, false
	}
	return movedBuildKeys[i], true
}

// takeMovedKeys reports and deletes the keys of the release: node that
// moved when the release extension got its version and build slots: an
// engine key of the section itself, and a project's build: mapping (each
// of its keys with its new place), so neither is decoded nor reported as
// an unknown field.
func takeMovedKeys(section *yaml.Node) []string {
	if section == nil || section.Kind != yaml.MappingNode {
		return nil
	}
	var problems []string
	kept := section.Content[:0:0]
	for j := 0; j+1 < len(section.Content); j += 2 {
		key := section.Content[j].Value
		if !slices.Contains(MovedEngineKeys, key) {
			kept = append(kept, section.Content[j], section.Content[j+1])
			continue
		}
		problems = append(problems, fmt.Sprintf("%s.%s: moved to %s.providers.%s.%s, the keys of the version engine (nerv migrate moves it)", SectionKey, key, SectionKey, EngineProvider, key))
	}
	section.Content = kept

	projects := mappingValue(section, "projects")
	if projects == nil || projects.Kind != yaml.SequenceNode {
		return problems
	}
	for i, p := range projects.Content {
		if p.Kind != yaml.MappingNode {
			continue
		}
		at := fmt.Sprintf("%s.projects[%d]", SectionKey, i)
		kept := p.Content[:0:0]
		for j := 0; j+1 < len(p.Content); j += 2 {
			key, value := p.Content[j], p.Content[j+1]
			if key.Value != "build" || value.Kind != yaml.MappingNode {
				kept = append(kept, key, value)
				continue
			}
			for k := 0; k+1 < len(value.Content); k += 2 {
				sub := value.Content[k].Value
				msg := "the build: mapping was replaced by the build provider's name (build: docker-compose or build: bash) and its keys"
				if move, ok := movedBuildKey(sub); ok {
					msg = move.message
				}
				problems = append(problems, fmt.Sprintf("%s.build.%s: %s", at, sub, fmt.Sprintf(msg, at)))
			}
			if len(value.Content) == 0 {
				problems = append(problems, fmt.Sprintf("%s.build: the build provider's name now (build: docker-compose or build: bash), not a mapping", at))
			}
		}
		p.Content = kept
	}
	return problems
}

// MoveKeys rewrites in place a release: section node written before the
// version and build slots (the 5.0.0-alpha.1 ship: section) into today's
// shape, moving each key where Load says it went (takeMovedKeys): the
// version engine keys of the section under
// providers.conventional-commits, and each project's build: mapping to
// build: docker-compose (mode compose) or build: bash (mode script), its
// file and test_service under the project's providers.docker-compose, its
// shared_host under providers.bash, and its images, version_files and
// coverage to the project itself. Every other key and comment is kept. It
// returns one note per moved key, none when there is nothing to move, and
// an error naming what has no place: an unknown build mode or build key,
// or a key its new place already has.
func MoveKeys(section *yaml.Node) ([]string, error) {
	if section == nil || section.Kind != yaml.MappingNode {
		return nil, nil
	}
	var notes []string
	var engine []*yaml.Node
	at := -1
	kept := section.Content[:0:0]
	for j := 0; j+1 < len(section.Content); j += 2 {
		key := section.Content[j].Value
		if !slices.Contains(MovedEngineKeys, key) {
			kept = append(kept, section.Content[j], section.Content[j+1])
			continue
		}
		if at < 0 {
			at = len(kept)
		}
		engine = append(engine, section.Content[j], section.Content[j+1])
		notes = append(notes, fmt.Sprintf("%s.%s: moved to %s.providers.%s.%s", SectionKey, key, SectionKey, EngineProvider, key))
	}
	if len(engine) > 0 {
		section.Content = kept
		block, err := providerBlock(section, at, EngineProvider, SectionKey)
		if err != nil {
			return nil, err
		}
		if err := addKeys(block, engine, SectionKey+".providers."+EngineProvider); err != nil {
			return nil, err
		}
	}

	projects := mappingValue(section, "projects")
	if projects == nil || projects.Kind != yaml.SequenceNode {
		return notes, nil
	}
	for i, p := range projects.Content {
		moved, err := moveBuild(p, fmt.Sprintf("%s.projects[%d]", SectionKey, i))
		if err != nil {
			return nil, err
		}
		notes = append(notes, moved...)
	}
	return notes, nil
}

// moveBuild is MoveKeys for one project node (at is its path).
func moveBuild(p *yaml.Node, at string) ([]string, error) {
	if p.Kind != yaml.MappingNode {
		return nil, nil
	}
	idx := -1
	for j := 0; j+1 < len(p.Content); j += 2 {
		if p.Content[j].Value == "build" && p.Content[j+1].Kind == yaml.MappingNode {
			idx = j
		}
	}
	if idx < 0 {
		return nil, nil
	}
	build := p.Content[idx+1]
	var notes []string
	var own []*yaml.Node                    // the keys that go to the project itself
	byProvider := map[string][]*yaml.Node{} // the keys that go under providers.<p>
	var providerOrder []string
	provider := ""
	for k := 0; k+1 < len(build.Content); k += 2 {
		key, value := build.Content[k], build.Content[k+1]
		move, ok := movedBuildKey(key.Value)
		switch {
		case !ok:
			return nil, fmt.Errorf("%s.build.%s: unknown build key (want %s): move it by hand", at, key.Value, strings.Join(buildKeyNames(), ", "))
		case move.key == "mode":
			provider, ok = buildModes[value.Value]
			if !ok {
				return nil, fmt.Errorf("%s.build.mode: unknown build mode %q (want compose or script): set %s.build to docker-compose or bash by hand", at, value.Value, at)
			}
			notes = append(notes, fmt.Sprintf("%s.build.mode: %s became %s.build: %s", at, value.Value, at, provider))
		case move.provider != "":
			if !slices.Contains(providerOrder, move.provider) {
				providerOrder = append(providerOrder, move.provider)
			}
			byProvider[move.provider] = append(byProvider[move.provider], key, value)
			notes = append(notes, fmt.Sprintf("%s.build.%s: moved to %s.providers.%s.%s", at, key.Value, at, move.provider, key.Value))
		default:
			own = append(own, key, value)
			notes = append(notes, fmt.Sprintf("%s.build.%s: moved to %s.%s", at, key.Value, at, key.Value))
		}
	}
	for j := 0; j < len(own); j += 2 {
		if mappingValue(p, own[j].Value) != nil {
			return nil, fmt.Errorf("%s.build.%s: %s.%s is set too: keep one by hand", at, own[j].Value, at, own[j].Value)
		}
	}
	// build: is replaced in place by the provider's name (or dropped
	// without a mode), followed by the keys that moved to the project.
	head := p.Content[idx]
	var content []*yaml.Node
	content = append(content, p.Content[:idx]...)
	if provider != "" {
		name := scalarNode(provider)
		name.LineComment = build.LineComment
		content = append(content, head, name)
	}
	content = append(content, own...)
	after := len(content)
	content = append(content, p.Content[idx+2:]...)
	p.Content = content
	for _, name := range providerOrder {
		block, err := providerBlock(p, after, name, at)
		if err != nil {
			return nil, err
		}
		if err := addKeys(block, byProvider[name], at+".providers."+name); err != nil {
			return nil, err
		}
	}
	// A project written as a flow mapping becomes a block one: its build
	// keys spread over its providers.
	p.Style = 0
	return notes, nil
}

// buildKeyNames are the keys a build: mapping could hold.
func buildKeyNames() []string {
	var out []string
	for _, m := range movedBuildKeys {
		out = append(out, m.key)
	}
	return out
}

// providerBlock returns the providers.<name> mapping of m (at is m's path),
// creating it, and providers: at index at of m's content when m has none.
func providerBlock(m *yaml.Node, at int, name, path string) (*yaml.Node, error) {
	providers := mappingValue(m, "providers")
	switch {
	case providers == nil:
		providers = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if at < 0 || at > len(m.Content) {
			at = len(m.Content)
		}
		m.Content = slices.Insert(m.Content, at, scalarNode("providers"), providers)
	case providers.ShortTag() == "!!null":
		*providers = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	case providers.Kind != yaml.MappingNode:
		return nil, fmt.Errorf("%s.providers: must be a mapping", path)
	}
	block := mappingValue(providers, name)
	switch {
	case block == nil:
		block = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		providers.Content = append(providers.Content, scalarNode(name), block)
	case block.ShortTag() == "!!null":
		*block = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	case block.Kind != yaml.MappingNode:
		return nil, fmt.Errorf("%s.providers.%s: must be a mapping", path, name)
	}
	return block, nil
}

// addKeys appends the key/value pairs kv to the mapping block (at is its
// path), refusing a key it already has.
func addKeys(block *yaml.Node, kv []*yaml.Node, at string) error {
	for j := 0; j+1 < len(kv); j += 2 {
		if mappingValue(block, kv[j].Value) != nil {
			return fmt.Errorf("%s.%s is set too: keep one by hand", at, kv[j].Value)
		}
		block.Content = append(block.Content, kv[j], kv[j+1])
	}
	return nil
}
