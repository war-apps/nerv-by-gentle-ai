// Package taskprovider is the task extension's side of the provider
// contract: the remote operations of its provider slot, the canonical
// stage keys, the pipeline events with the operations each needs, and the
// task fields of a task provider's provider.yaml (task_ref_prefix, stages,
// events). The shape every provider shares (name, extension, slot,
// runtime, mcp, config, operations) is internal/extension's; this package
// adds what only task providers mean. The prose side of the contract lives
// in plugins/task/skills/task/SKILL.md.
package taskprovider

import "github.com/war-apps/nerv-by-gentle-ai/internal/extension"

// ContractVersion is the only contract_version this build understands.
const ContractVersion = extension.ContractVersion

// NotImplemented declares an operation the provider does not support. An
// event that needs it makes the manifest invalid.
const NotImplemented = extension.NotImplemented

// Extension and Slot are where a task provider plugs in: the provider
// slot of the task extension (plugins/task/extension.yaml).
const (
	Extension = "task"
	Slot      = "provider"
)

// Operations are the remote primitives a provider implements, the
// operations of the task extension's provider slot. Timers, timelog
// consolidation and the composite operations (block, cancel, close, done,
// stopAll) belong to the task plugin and are built from these.
var Operations = []string{
	"getTask",
	"assign",
	"moveStage",
	"logTime",
	"updateTimelog",
	"deleteTimelog",
	"listTimelogs",
	"createTask",
	"createSubtask",
	"comment",
	"complete",
	"setPriority",
	"listTasks",
}

// StageKeys are the canonical stages events speak; a provider maps each
// one to its own stage name through its stages block and config.
var StageKeys = []string{"pending", "analysis", "inDev", "testing", "implemented", "blocked", "canceled"}

// Results are the outcomes the dispatcher reports per provider for one
// event: the statuses of the common response envelope. needs_input carries
// a question the orchestrator relays verbatim before dispatching again.
var Results = extension.Statuses

// Events maps each event the pipeline emits to the operations a provider
// must implement to handle it, as the provider slot's events in
// plugins/task/extension.yaml declare it (the shared validator enforces
// that table). run.closed never completes the task: that stays an
// explicit user action (/task:close).
var Events = map[string][]string{
	"run.started":          {"getTask", "assign", "moveStage"},
	"verification.started": {"moveStage"},
	"audit.ranked":         {"comment"},
	"issue.deferred":       {"createTask"},
	"wave.started":         {"createSubtask"},
	"run.closed":           {"moveStage", "logTime"},
	"run.halted":           {"comment", "moveStage", "logTime"},
}
