// Package taskprovider holds the contract between the task plugin and the
// provider plugins it dispatches to: the events the NERV pipeline emits,
// the remote operations a provider implements, the canonical stage keys,
// and the provider.yaml manifest each provider ships. The prose side of the
// contract lives in plugins/task/skills/task/SKILL.md; this package is the
// part the nerv CLI can check mechanically.
package taskprovider

// ContractVersion is the only contract_version this build understands.
const ContractVersion = 1

// NotImplemented declares an operation the provider does not support. An
// event that needs it makes the manifest invalid.
const NotImplemented = "not_implemented"

// Operations are the remote primitives a provider implements. Timers,
// timelog consolidation and the composite operations (block, cancel,
// close, done, stopAll) belong to the task plugin and are built from these.
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

// Results are the outcomes a provider reports for one event.
var Results = []string{"ok", "skipped", "blocked", "failed"}

// Events maps each event the pipeline emits to the operations a provider
// must implement to handle it. run.closed never completes the task: that
// stays an explicit user action (/task:close).
var Events = map[string][]string{
	"run.started":          {"getTask", "assign", "moveStage"},
	"verification.started": {"moveStage"},
	"audit.ranked":         {"comment"},
	"issue.deferred":       {"createTask"},
	"wave.started":         {"createSubtask"},
	"run.closed":           {"moveStage", "logTime"},
	"run.halted":           {"comment", "moveStage", "logTime"},
}
