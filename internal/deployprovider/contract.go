// Package deployprovider holds the contract between `nerv deploy` and the
// provider plugins it runs: a git host (the release on GitHub, GitLab, ...)
// or a deploy target (a registry, a server). Each provider plugin ships a
// deploy-provider.yaml manifest declaring its kind, its config keys, the
// executables it needs and its operations.
//
// An operation is one or more argv templates that Go runs directly, never
// through a shell. A template element is either a literal or exactly one
// {variable}, substituted whole as one argument; the variables come from
// the artifact's manifest, the provider's config and nerv itself, so the
// release notes or any other value can never inject a command. Each step
// gets a JSON Request on its stdin and may print a JSON Response on its
// stdout.
package deployprovider

// ContractVersion is the only contract_version this build understands.
const ContractVersion = 1

// ManifestFile is the manifest's name at the root of a deploy provider
// plugin. It is not provider.yaml, which marks a task provider.
const ManifestFile = "deploy-provider.yaml"

// The provider kinds.
const (
	KindGitHost = "git-host"
	KindTarget  = "target"
)

// Kinds are the provider kinds.
var Kinds = []string{KindGitHost, KindTarget}

// The operations.
const (
	OpReleaseExists = "releaseExists"
	OpCreateRelease = "createRelease"
	OpPublish       = "publish"
	OpDeploy        = "deploy"
	OpVerify        = "verify"
)

// Operations maps each kind to the operations its providers implement, in
// the order deploy runs them.
var Operations = map[string][]string{
	KindGitHost: {OpReleaseExists, OpCreateRelease},
	KindTarget:  {OpPublish, OpDeploy, OpVerify},
}

// NotImplemented declares an optional operation the provider does not
// support (only a target's publish and verify are optional).
const NotImplemented = "not_implemented"

// Optional are the operations a provider may declare NotImplemented.
var Optional = []string{OpPublish, OpVerify}

// ForEach are the lists a step can run once per item of, with {item}:
// images (the manifest's image references), image_tars (the absolute paths
// of the image-tar outputs) and outputs (the absolute paths of every
// output).
var ForEach = []string{"images", "image_tars", "outputs"}

// Variables are the template variables every operation can use, besides
// config.<key> (a key the manifest declares) and item (in a for_each
// step). prerelease is "true" for a release on a channel, "false"
// otherwise.
var Variables = []string{
	"project", "version", "channel", "tag", "base_commit", "branch", "build_mode",
	"prerelease", "artifact_dir", "notes_file", "changelog_file", "target", "nerv",
}

// Booleans are the variables a step's or an argument's when can test.
var Booleans = []string{"prerelease"}

// Request is what every step reads on its stdin.
type Request struct {
	ContractVersion int    `json:"contract_version"`
	Operation       string `json:"operation"`
	// Target is the deploy.targets name; empty for the git host.
	Target string `json:"target,omitempty"`
	// Artifact is the artifact's manifest (artifact.Manifest).
	Artifact    any    `json:"artifact"`
	ArtifactDir string `json:"artifact_dir"`
	// Config is the provider's resolved config.
	Config map[string]string `json:"config"`
	// Item is the for_each item, when the step has one.
	Item string `json:"item,omitempty"`
}

// The statuses a Response can report.
const (
	StatusOK      = "ok"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
)

// Response is what a step may print on its stdout. A step printing no
// JSON object succeeds or fails by its exit code alone.
type Response struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	// Exists answers releaseExists.
	Exists *bool `json:"exists,omitempty"`
}
