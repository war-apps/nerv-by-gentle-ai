package shipconfig

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig/yamlsection"
	"gopkg.in/yaml.v3"
)

// MigrateHint is how a repository moves from the removed sections to the
// ship: and deploy: sections.
const MigrateHint = "nerv migrate rewrites it; by hand, see /ship:setup and /deploy:setup"

// RemovedSections reports the removed top-level sections of a
// .ai-workflow.yml text, each with what replaced it: release: (renamed to
// ship:, its semantic-release engine replaced by the native one) and
// pipeline: (nerv pipeline was removed), naming where each of its parts went.
// Nil when the text has neither. Every reader of the ship: and deploy:
// sections refuses a file with one, so the old configuration is never
// silently ignored.
func RemovedSections(data []byte) []string {
	var out []string
	if present(data, "release") {
		out = append(out, "release: the section is no longer read: it is now the ship: section, whose version engine is native "+
			"(no semantic-release, node or npm). Rename the key to ship:, drop semantic_release:, give every project its build.mode, "+
			"and import the branches and release rules of your semantic-release configuration with nerv ship setup --import; "+MigrateHint)
	}
	if msg := RemovedPipeline(data); msg != "" {
		out = append(out, msg)
	}
	return out
}

// RemovedPipeline is RemovedSections' message for the pipeline: section
// ("" without one): the deploy: section's reader refuses it too.
func RemovedPipeline(data []byte) string {
	if node, ok := locate(data, "pipeline"); ok {
		return pipelineMessage(node)
	}
	return ""
}

// present reports whether data has the top-level key, even when its block
// cannot be parsed.
func present(data []byte, key string) bool {
	_, ok := locate(data, key)
	return ok
}

// locate returns the key's value node (nil when it cannot be read) and
// whether the key is there.
func locate(data []byte, key string) (*yaml.Node, bool) {
	node, err := yamlsection.Locate(data, key)
	var problem *yamlsection.ProblemError
	switch {
	case errors.Is(err, yamlsection.ErrAbsent), errors.As(err, &problem):
		// A problem with the whole document (its top level is not a
		// mapping) is the section reader's to report.
		return nil, false
	case err != nil:
		// The block could not be parsed: the key is there only when a
		// line says so.
		return nil, regexp.MustCompile(`(?m)^["']?` + regexp.QuoteMeta(key) + `["']?[ \t]*:`).Match(data)
	}
	return node, true
}

// pipelineParts are the pipeline: section's steps, in its run order, with
// where each one went.
var pipelineParts = []struct {
	keys []string
	text string
}{
	{[]string{"validate", "test"}, "validate and test: nerv ship (stages of each project in the ship: section)"},
	{[]string{"docker", "release"}, "docker and release: nerv ship, then nerv deploy (the image is built and saved into the artifact by ship and pushed by deploy)"},
	{[]string{"deploy"}, "deploy: the deploy-ssh-compose target of the deploy: section"},
	{[]string{"sync"}, "sync: not ported yet (no back-merge after a release)"},
}

// pipelineMessage names where every part of the pipeline: section went: the
// parts it has, or all of them when none can be read.
func pipelineMessage(node *yaml.Node) string {
	var keys []string
	if node != nil && node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			keys = append(keys, node.Content[i].Value)
		}
	}
	var parts []string
	for _, p := range pipelineParts {
		if slices.ContainsFunc(p.keys, func(k string) bool { return slices.Contains(keys, k) }) {
			parts = append(parts, p.text)
		}
	}
	if len(parts) == 0 {
		for _, p := range pipelineParts {
			parts = append(parts, p.text)
		}
	}
	msg := "pipeline: the pipeline: section is no longer read (nerv pipeline was removed): " + strings.Join(parts, "; ") + "."
	if deploy := mappingValue(node, "deploy"); deploy != nil {
		if sel := mappingValue(deploy, "selection"); sel != nil && sel.Value == "branches" {
			msg += " pipeline.deploy.selection: branches has no equivalent: a deploy-ssh-compose target deploys its environment when the environment's " +
				"DOCKER_IMAGE_VERSION points at the release; list one target per environment and choose them with nerv deploy --target."
		}
	}
	return msg + " Remove the section; " + MigrateHint + "."
}

// removedSectionKeys are the keys removed from the section itself.
var removedSectionKeys = []removedKey{
	{"semantic_release", "removed: the version is computed natively (no semantic-release, node or npm): delete it; " +
		"import the branches and release rules of your semantic-release configuration with nerv ship setup --import"},
}

// takeRemovedSectionKeys reports and deletes the removed keys of the ship:
// node itself.
func takeRemovedSectionKeys(section *yaml.Node) []string {
	if section == nil || section.Kind != yaml.MappingNode {
		return nil
	}
	var problems []string
	kept := section.Content[:0:0]
	for j := 0; j+1 < len(section.Content); j += 2 {
		key := section.Content[j].Value
		idx := slices.IndexFunc(removedSectionKeys, func(r removedKey) bool { return r.key == key })
		if idx < 0 {
			kept = append(kept, section.Content[j], section.Content[j+1])
			continue
		}
		problems = append(problems, fmt.Sprintf("%s.%s: %s", SectionKey, key, removedSectionKeys[idx].message))
	}
	section.Content = kept
	return problems
}
