// Package extension holds the two contracts every NERV extension and
// provider speaks, whatever it does:
//
//   - core <-> extension: the extension.yaml manifest at an extension's
//     root (its runtime, the core events it subscribes to, its config keys
//     and the slots its providers fill);
//   - extension <-> provider: the provider.yaml manifest at a provider's
//     root (the extension and slot it fills, its runtime, its config keys
//     and one entry per slot operation).
//
// Both share the config key schema, the name rules, ContractVersion and
// the Request/Response envelope an operation is called with. An extension
// adds its own semantics on top (the task events a provider handles, the
// deploy step variables) in its own package; this package only checks what
// every extension and provider has in common. docs/extensions.md documents
// both manifests.
package extension

import (
	"bytes"
	"encoding/json"
	"regexp"
)

// ContractVersion is the only contract_version this build understands, in
// extension.yaml and provider.yaml alike.
const ContractVersion = 1

// The manifest files, at the root of an extension or a provider folder.
const (
	ManifestFile = "extension.yaml"
	ProviderFile = "provider.yaml"
)

// NotImplemented declares a slot operation the provider does not support;
// only an optional operation may be declared so.
const NotImplemented = "not_implemented"

// The runtimes: exec runs a command, shell-free, with the JSON Request on
// its stdin and an optional JSON Response on its stdout; agent has an agent
// follow procedures (an extension's entry agent, a provider's procedure
// files) with MCP tools.
const (
	RuntimeAgent = "agent"
	RuntimeExec  = "exec"
)

// Runtimes are the runtimes a manifest may declare.
var Runtimes = []string{RuntimeAgent, RuntimeExec}

// The slot cardinalities: one provider at a time (deploy's git host), any
// number active at once (task providers), or one per configured target
// (deploy targets).
const (
	CardinalityOne       = "one"
	CardinalityMany      = "many"
	CardinalityPerTarget = "per-target"
)

// Cardinalities are the cardinalities a slot may declare.
var Cardinalities = []string{CardinalityOne, CardinalityMany, CardinalityPerTarget}

// How nerv install installs an extension's providers (extension.yaml's
// providers): all of them with the extension, or each one offered after
// it.
const (
	ProvidersBundled   = "bundled"
	ProvidersSuggested = "suggested"
)

// ProvidersModes are the values extension.yaml's providers may take.
var ProvidersModes = []string{ProvidersBundled, ProvidersSuggested}

// The operation requirement levels of a slot.
const (
	Required = "required"
	Optional = "optional"
)

// CoreEvents are the events the NERV pipeline emits, which an extension
// may subscribe to in its extension.yaml.
var CoreEvents = []string{
	"run.started",
	"verification.started",
	"audit.ranked",
	"issue.deferred",
	"wave.started",
	"run.closed",
	"run.halted",
}

// The statuses a Response reports.
const (
	StatusOK         = "ok"
	StatusSkipped    = "skipped"
	StatusBlocked    = "blocked"
	StatusFailed     = "failed"
	StatusNeedsInput = "needs_input"
)

// Statuses are every status a Response may report, in contract order.
var Statuses = []string{StatusOK, StatusSkipped, StatusBlocked, StatusFailed, StatusNeedsInput}

// Request is what an operation is called with: the JSON object an exec
// runtime reads on its stdin, and the fields an agent runtime is handed.
type Request struct {
	// Contract is ContractVersion.
	Contract  int    `json:"contract"`
	Extension string `json:"extension"`
	Slot      string `json:"slot,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Operation string `json:"operation"`
	// Event is the core event being handled, when there is one.
	Event string `json:"event,omitempty"`
	// Config is the provider's (or extension's) resolved config.
	Config map[string]any `json:"config"`
	// Payload is the extension-specific input of the operation.
	Payload any `json:"payload,omitempty"`
}

// Response is what an operation answers: an exec runtime prints it as one
// JSON object on its stdout (printing none answers by exit code alone).
type Response struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	// Data is the operation's extension-specific answer.
	Data json.RawMessage `json:"data,omitempty"`
}

// Failure reports whether the status stops the caller: failed, and the
// blocked and needs_input statuses a caller that cannot ask the user (such
// as nerv deploy) treats as failures too.
func (r Response) Failure() bool {
	return r.Status == StatusFailed || r.Status == StatusBlocked || r.Status == StatusNeedsInput
}

// DecodeData decodes Data into v; no Data leaves v untouched.
func (r Response) DecodeData(v any) error {
	if len(r.Data) == 0 {
		return nil
	}
	return json.Unmarshal(r.Data, v)
}

// ParseResponse reads a JSON object response from an exec runtime's
// stdout; anything else is no response (ok false).
func ParseResponse(stdout []byte) (Response, bool) {
	trimmed := bytes.TrimSpace(stdout)
	var r Response
	if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(trimmed, &r) != nil {
		return Response{}, false
	}
	return r, true
}

var (
	nameRe  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	keyRe   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	opRe    = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	toolRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
	agentRe = regexp.MustCompile(`^[a-z][a-z0-9-]*:[a-z][a-z0-9-]*$`)
)

// ValidName reports whether name is an extension, provider or slot name:
// lower-case letters, digits and dashes, starting with a letter.
func ValidName(name string) bool { return nameRe.MatchString(name) }
