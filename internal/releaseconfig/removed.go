package releaseconfig

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig/yamlsection"
	"gopkg.in/yaml.v3"
)

// MigrateHint is how a repository moves from the removed sections to the
// release: and deploy: sections.
const MigrateHint = "nerv migrate rewrites it; by hand, see /release:setup and /deploy:setup"

// RemovedSections reports the removed top-level sections of a
// .ai-workflow.yml text, each with what replaced it: pipeline: (nerv
// pipeline was removed), naming where each of its parts went. Nil when the
// text has none. Every reader of the release: and deploy: sections refuses
// a file with one, so the old configuration is never silently ignored. A
// 4.2.0 release: section has the same key as today's: it is told apart by
// its keys (LegacyKeys) and refused by Load.
func RemovedSections(data []byte) []string {
	var out []string
	if msg := RemovedPipeline(data); msg != "" {
		out = append(out, msg)
	}
	return out
}

// LegacyHint is how a 4.2.0 release: section becomes today's.
const LegacyHint = "run nerv migrate to convert it into the current release: section"

// LegacyKeys returns the 4.2.0 keys of the release: section of a
// .ai-workflow.yml text, each with its path (release.semantic_release,
// release.projects[0].presets): the keys only the 4.2.0 section had, which
// tell it apart from today's. Nil when the section has none or cannot be
// read.
func LegacyKeys(data []byte) []string {
	node, ok := locate(data, SectionKey)
	if !ok {
		return nil
	}
	return legacyKeys(node)
}

// legacyKeys is LegacyKeys over the section's node.
func legacyKeys(section *yaml.Node) []string {
	if section == nil || section.Kind != yaml.MappingNode {
		return nil
	}
	var out []string
	for j := 0; j+1 < len(section.Content); j += 2 {
		if key := section.Content[j].Value; slices.ContainsFunc(removedSectionKeys, func(r removedKey) bool { return r.key == key }) {
			out = append(out, SectionKey+"."+key)
		}
	}
	projects := mappingValue(section, "projects")
	if projects == nil || projects.Kind != yaml.SequenceNode {
		return out
	}
	for i, p := range projects.Content {
		if p.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(p.Content); j += 2 {
			if key := p.Content[j].Value; slices.ContainsFunc(removedProjectKeys, func(r removedKey) bool { return r.key == key }) {
				out = append(out, fmt.Sprintf("%s.projects[%d].%s", SectionKey, i, key))
			}
		}
	}
	return out
}

// legacyMessage is the refusal of a section with 4.2.0 keys ("" without
// any).
func legacyMessage(section *yaml.Node) string {
	keys := legacyKeys(section)
	if len(keys) == 0 {
		return ""
	}
	return fmt.Sprintf("%s: this is a 4.2.0 release: section (it has %s): %s; by hand, see /release:setup", SectionKey, strings.Join(keys, ", "), LegacyHint)
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

// pipelineParts are the pipeline: section's 4.2.0 keys, in its run order,
// with where each one went. The validate, test and docker entries cover
// hand-written sections that named the steps instead of the keys.
var pipelineParts = []struct {
	keys []string
	text string
}{
	{[]string{"dotnet"}, "pipeline.dotnet: the stages (restore, lint, build, test) of a project with build: bash in the release: section"},
	{[]string{"validate", "test"}, "validate and test: nerv release (stages of each project in the release: section)"},
	{[]string{"branches"}, "pipeline.branches: release.providers.conventional-commits.branches (the version engine's branches)"},
	{[]string{"registry", "images"}, "pipeline.registry and pipeline.images: the project's images (built and saved into the artifact by nerv release), " +
		"pushed by the deploy-ssh-compose target's publish (the registry login is its config)"},
	{[]string{"docker"}, "docker and release: nerv release, then nerv deploy (the image is built and saved into the artifact by release and pushed by deploy)"},
	{[]string{"release"}, "pipeline.release.version_files: the project's version_files; the release itself is nerv release, then nerv deploy (tag, release commit and host release)"},
	{[]string{"deploy"}, "pipeline.deploy: the deploy-ssh-compose targets of the deploy: section (one per environment)"},
	{[]string{"sync"}, "pipeline.sync: not ported yet (no back-merge after a release)"},
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
	return msg + " To translate it, run nerv migrate (" + MigrateHint + "); then remove the section."
}

// removedSectionKeys are the keys removed from the section itself.
var removedSectionKeys = []removedKey{
	{"semantic_release", "removed: the version is computed natively (no semantic-release, node or npm): delete it; " +
		"import the branches and release rules of your semantic-release configuration with nerv release setup --import"},
}

// takeRemovedSectionKeys reports and deletes the removed keys of the release:
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
