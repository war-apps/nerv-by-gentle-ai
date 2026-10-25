package releaseplan

import "github.com/war-apps/nerv-by-gentle-ai/internal/versioning"

// The release extension's side of the provider contract
// (plugins/release/extension.yaml, docs/extensions.md): its two slots,
// their operations, and the payload each operation is called with and the
// data it answers. Every call is the common envelope (extension.Request
// and extension.Response); a provider's config is its resolved
// release.providers.<provider> keys (a build provider's overridden by
// release.projects[].providers.<provider>).

// Extension is the release extension's name.
const Extension = "release"

// The slots: the version engine (one for the section) and the build (one
// per project: release.build, overridden by release.projects[].build).
const (
	SlotVersion = "version"
	SlotBuild   = "build"
)

// The operations. validate and import are optional; the others required.
const (
	// OpValidate checks the provider's config (and, for the version slot,
	// the projects; for the build slot, the project's stages) before
	// anything runs: it answers failed with ProblemsData.
	OpValidate = "validate"
	// OpNext computes each project's next release on a branch (NextData).
	OpNext = "next"
	// OpBranch resolves a branch against the configured release branches
	// (BranchData).
	OpBranch = "branch"
	// OpImport reads the repository's own configuration of the engine (a
	// semantic-release one) into the provider's keys (ImportData).
	OpImport = "import"
	// OpPlan gives a project's build steps (PlanData).
	OpPlan = "plan"
)

// VersionProject is one project as the version engine reads it: the
// identity keys of a release.projects entry.
type VersionProject struct {
	Name        string              `json:"name"`
	Path        string              `json:"path"`
	SharedPaths []string            `json:"shared_paths,omitempty"`
	Scopes      []string            `json:"scopes,omitempty"`
	TagFormat   string              `json:"tag_format,omitempty"`
	Changelog   string              `json:"changelog,omitempty"`
	DependsOn   []VersionDependency `json:"depends_on,omitempty"`
}

// VersionDependency is a project's depends_on entry.
type VersionDependency struct {
	Project string `json:"project"`
	Bump    string `json:"bump,omitempty"`
}

// VersionPayload is the payload of every version operation.
type VersionPayload struct {
	// Dir is the repository (the release clone) the operation reads;
	// empty for validate.
	Dir string `json:"dir,omitempty"`
	// Branch is the branch computed (next, branch).
	Branch string `json:"branch,omitempty"`
	// Projects are the section's projects; none versions the whole
	// repository as one project.
	Projects []VersionProject `json:"projects"`
	// Only restricts next's results to these projects (their dependencies
	// are still computed); empty means every project.
	Only []string `json:"only,omitempty"`
	// Version is an explicit version (nerv release --version) next
	// releases instead of computing one.
	Version string `json:"version,omitempty"`
}

// NextData answers next: one result per project, in the engine's order.
type NextData struct {
	Results []versioning.Result `json:"results"`
}

// BranchData answers branch: the branch's prerelease identifier and
// channel (both empty on a stable release branch of the default channel).
type BranchData struct {
	Name       string `json:"name"`
	Prerelease string `json:"prerelease,omitempty"`
	Channel    string `json:"channel,omitempty"`
}

// ProblemsData answers a failed validate: every problem, each prefixed by
// its path in .ai-workflow/config.yaml (release.providers.<provider>.<key>,
// release.projects[i].<key>).
type ProblemsData struct {
	Problems []string `json:"problems"`
}

// ImportData answers import.
type ImportData struct {
	// Source is the file the configuration was read from.
	Source string `json:"source,omitempty"`
	// Config is the imported keys as a YAML mapping, in the provider's key
	// order, to write under release.providers.<provider>.
	Config string `json:"config,omitempty"`
	// Warnings are the parts that could not be translated.
	Warnings []string `json:"warnings,omitempty"`
	// Absent is set when there is no configuration to import (status
	// skipped); Manual names a configuration that must be moved by hand,
	// such as a JavaScript one (status blocked).
	Absent bool   `json:"absent,omitempty"`
	Manual string `json:"manual,omitempty"`
}

// BuildPayload is the payload of the build operations.
type BuildPayload struct {
	// Project is the project's name, and At its path in .ai-workflow/config.yaml
	// (release.projects[0]), for messages.
	Project string `json:"project"`
	At      string `json:"at"`
	// Stages are the pipeline stages the project writes its own commands
	// for (release.projects[].stages), in pipeline order.
	Stages []string `json:"stages"`
}

// PlanData answers plan: the provider's own steps and how the project's
// commands run.
type PlanData struct {
	// Steps are the provider's built-in commands, each in its stage, in
	// order; they run before the project's command of the stage.
	Steps []BuildStep `json:"steps,omitempty"`
	// ImagePrefix starts the name of the image steps of the package stage
	// (docker tag and docker save of release.projects[].images), such as
	// "compose:".
	ImagePrefix string `json:"image_prefix"`
	// StageEnv is added, per stage, under the env of the project's own
	// command of that stage (the command's own variables win).
	StageEnv map[string]map[string]string `json:"stage_env,omitempty"`
	// Paths are the repository files the plan reads, by the config key
	// naming them: nerv release checks that origin's branch has them.
	Paths map[string]string `json:"paths,omitempty"`
}

// BuildStep is one built-in command of a build provider, run without a
// shell in the release clone.
type BuildStep struct {
	Stage string   `json:"stage"`
	Name  string   `json:"name"`
	Exec  []string `json:"exec"`
}
