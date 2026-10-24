package releaserun

import (
	"slices"
	"strconv"
	"strings"
)

// EnvPrefix is the prefix of the variables NERV passes to every command;
// releaseconfig rejects it in a command's env.
const EnvPrefix = "NERV_RELEASE_"

// Vars are the values of the NERV_RELEASE_* variables for one project's
// release, supplied by the caller (nerv release). The stage and the repository
// root come from the Step and the Runner.
type Vars struct {
	Project     string
	ProjectPath string
	Version     string
	Channel     string
	Tag         string
	GitHead     string
	LastVersion string
	NotesFile   string
	Branch      string
	DryRun      bool
}

// envVar is one NERV_RELEASE_* variable and where its value comes from.
type envVar struct {
	name  string
	value func(v Vars, repoRoot, stage string) string
}

// envTable lists the variables every command receives, in the order they
// are set. Each is always set, empty when the value is unknown at that step.
var envTable = []envVar{
	// Project name.
	{EnvPrefix + "PROJECT", func(v Vars, _, _ string) string { return v.Project }},
	// Project path, repository-relative, forward slashes ("." for the root).
	{EnvPrefix + "PROJECT_PATH", func(v Vars, _, _ string) string {
		if v.ProjectPath == "" {
			return "."
		}
		return v.ProjectPath
	}},
	// Absolute repository root.
	{EnvPrefix + "REPO_ROOT", func(_ Vars, root, _ string) string { return root }},
	// Stage or hook key: test, before_test, prepare, ...
	{EnvPrefix + "STAGE", func(_ Vars, _, stage string) string { return stage }},
	// Next version, e.g. 1.2.0-alpha.2.
	{EnvPrefix + "VERSION", func(v Vars, _, _ string) string { return v.Version }},
	// Release channel, empty for the default channel.
	{EnvPrefix + "CHANNEL", func(v Vars, _, _ string) string { return v.Channel }},
	// Next tag, e.g. api-v1.2.0.
	{EnvPrefix + "TAG", func(v Vars, _, _ string) string { return v.Tag }},
	// Commit the release is made from.
	{EnvPrefix + "GIT_HEAD", func(v Vars, _, _ string) string { return v.GitHead }},
	// Last release version on the channel, empty when none.
	{EnvPrefix + "LAST_VERSION", func(v Vars, _, _ string) string { return v.LastVersion }},
	// File holding the release notes (Markdown).
	{EnvPrefix + "NOTES_FILE", func(v Vars, _, _ string) string { return v.NotesFile }},
	// Branch being released.
	{EnvPrefix + "BRANCH", func(v Vars, _, _ string) string { return v.Branch }},
	// "true" for nerv release --dry-run, else "false".
	{EnvPrefix + "DRY_RUN", func(v Vars, _, _ string) string { return strconv.FormatBool(v.DryRun) }},
}

// EnvVars are the names of the variables every command receives, in the
// order they are set (see envTable).
var EnvVars = func() []string {
	names := make([]string, len(envTable))
	for i, v := range envTable {
		names[i] = v.name
	}
	return names
}()

// environment is the command's full environment: the inherited one
// without any NERV_RELEASE_* variable, then the command's env (sorted by
// name), then the NERV_RELEASE_* variables. os/exec keeps the last value of
// a repeated name, so the command's env overrides an inherited variable.
func (r *Runner) environment(extra map[string]string, vars Vars, stage string) []string {
	var env []string
	for _, kv := range r.environ() {
		if !strings.HasPrefix(strings.ToUpper(kv), EnvPrefix) {
			env = append(env, kv)
		}
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		env = append(env, k+"="+extra[k])
	}
	for _, v := range envTable {
		env = append(env, v.name+"="+v.value(vars, r.RepoRoot, stage))
	}
	return env
}
