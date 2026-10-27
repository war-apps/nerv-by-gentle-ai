// Package deployprovider is nerv deploy's side of the provider
// contract: the slots `nerv deploy` runs (a git host that makes the
// release on GitHub, GitLab, ...; deploy targets such as a registry or a
// server), their operations in run order, the step variables and the
// request payload. The shape every provider shares (name, extension,
// slot, runtime, requires, config, operations) is internal/extension's;
// a deploy provider is an exec provider of a host or target slot of
// plugins/release/extension.yaml.
//
// An operation is one or more argv templates that Go runs directly, never
// through a shell. A template element is either a literal or exactly one
// {variable}, substituted whole as one argument; the variables come from
// the artifact's manifest, the provider's config and nerv itself, so the
// release notes or any other value can never inject a command. Each step
// gets the common JSON request (extension.Request, its payload a Payload)
// on its stdin and may print the common JSON response on its stdout.
package deployprovider

import "github.com/war-apps/nerv-by-gentle-ai/internal/extension"

// ContractVersion is the only contract_version this build understands.
const ContractVersion = extension.ContractVersion

// ManifestFile is the manifest's name at the root of a deploy provider
// plugin, the provider.yaml every provider ships.
const ManifestFile = extension.ProviderFile

// Extension is the extension the deploy providers belong to: release,
// whose host and target slots are nerv deploy's.
const Extension = "release"

// The deploy slots of the release extension (plugins/release/extension.yaml).
const (
	SlotHost   = "host"
	SlotTarget = "target"
)

// Slots are the deploy slots.
var Slots = []string{SlotHost, SlotTarget}

// The operations.
const (
	OpReleaseExists = "releaseExists"
	OpCreateRelease = "createRelease"
	OpPreflight     = "preflight"
	OpPublish       = "publish"
	OpDeploy        = "deploy"
	OpVerify        = "verify"
)

// Operations maps each slot to its operations, in the order deploy runs
// them: a target's preflight before anything is written, every target's
// publish, then each target's deploy and verify.
var Operations = map[string][]string{
	SlotHost:   {OpReleaseExists, OpCreateRelease},
	SlotTarget: {OpPreflight, OpPublish, OpDeploy, OpVerify},
}

// NotImplemented declares an optional operation the provider does not
// support (a target's preflight, publish and verify).
const NotImplemented = extension.NotImplemented

// ForEach are the lists a step can run once per item of, with {item}:
// images (the manifest's image references), image_tars (the absolute paths
// of the image-tar outputs) and outputs (the absolute paths of every
// output).
var ForEach = []string{"images", "image_tars", "outputs"}

// Variables are the template variables every operation can use, besides
// config.<key> (a key the manifest declares) and item (in a for_each
// step). prerelease is "true" for a release on a channel, "false"
// otherwise; build_provider is the release build provider that made the
// artifact (its manifest's build_provider).
var Variables = []string{
	"project", "version", "channel", "tag", "base_commit", "branch", "build_provider",
	"prerelease", "artifact_dir", "notes_file", "changelog_file", "target", "nerv",
}

// Booleans are the variables a step's or an argument's when can test.
var Booleans = []string{"prerelease"}

// Payload is the deploy part of every step's request (the common
// envelope's payload).
type Payload struct {
	// Target is the deploy.targets name; empty for the git host.
	Target string `json:"target,omitempty"`
	// Artifact is the artifact's manifest (artifact.Manifest).
	Artifact    any    `json:"artifact"`
	ArtifactDir string `json:"artifact_dir"`
	// Item is the for_each item, when the step has one.
	Item string `json:"item,omitempty"`
}

// The statuses a response can report. nerv deploy cannot ask the user, so
// a blocked or needs_input answer fails the operation like failed.
const (
	StatusOK      = extension.StatusOK
	StatusSkipped = extension.StatusSkipped
	StatusFailed  = extension.StatusFailed
)

// The data a response can carry (its data object).
type (
	// ExistsData answers releaseExists: {"exists": false} when the release
	// is absent.
	ExistsData struct {
		Exists *bool `json:"exists"`
	}
	// PreflightData answers preflight: the executables the target's config
	// needs on PATH besides the provider's requires.
	PreflightData struct {
		Requires []string `json:"requires"`
	}
)
