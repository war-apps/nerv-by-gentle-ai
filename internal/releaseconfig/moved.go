package releaseconfig

import (
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"
)

// EngineProvider is the version engine whose keys the section held before
// the version slot (branches, release_rules, ...), and the slot's default:
// they now live under release.providers.<EngineProvider>.
const EngineProvider = "conventional-commits"

// MovedEngineKeys are the version engine keys the section held itself
// before they moved under release.providers.conventional-commits.
var MovedEngineKeys = []string{"branches", "release_rules", "note_keywords", "tag_format", "repository_url", "changelog"}

// movedBuildKeys are the keys of the build: mapping a project held before
// build became the build provider's name, with their new place (%s is the
// project's path, release.projects[i]).
var movedBuildKeys = map[string]string{
	"mode":          "build is the build provider's name now: %[1]s.build: docker-compose (was mode compose) or %[1]s.build: bash (was mode script)",
	"file":          "moved to %s.providers.docker-compose.file (or release.providers.docker-compose.file for every project)",
	"test_service":  "moved to %s.providers.docker-compose.test_service (or release.providers.docker-compose.test_service for every project)",
	"shared_host":   "moved to %s.providers.bash.shared_host (or release.providers.bash.shared_host for every project)",
	"images":        "moved to %s.images",
	"version_files": "moved to %s.version_files",
	"coverage":      "moved to %s.coverage",
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
				msg, ok := movedBuildKeys[sub]
				if !ok {
					msg = "the build: mapping was replaced by the build provider's name (build: docker-compose or build: bash) and its keys"
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
