package shiprun

import (
	"slices"
	"strconv"
	"strings"
)

// EnvPrefix is the prefix of the variables NERV passes to every command;
// shipconfig rejects it in a command's env.
const EnvPrefix = "NERV_RELEASE_"

// EnvVars are the variables every command receives, in the order they are
// set. Each is always set, empty when the value is unknown at that step.
var EnvVars = []string{
	EnvPrefix + "PROJECT",      // project name
	EnvPrefix + "PROJECT_PATH", // project path, repository-relative, forward slashes ("." for the root)
	EnvPrefix + "REPO_ROOT",    // absolute repository root
	EnvPrefix + "STAGE",        // stage or hook key: test, before_test, prepare, ...
	EnvPrefix + "VERSION",      // next version, e.g. 1.2.0-alpha.2
	EnvPrefix + "CHANNEL",      // release channel, empty for the default channel
	EnvPrefix + "TAG",          // next tag, e.g. api-v1.2.0
	EnvPrefix + "GIT_HEAD",     // commit the release is made from
	EnvPrefix + "LAST_VERSION", // last release version on the channel, empty when none
	EnvPrefix + "NOTES_FILE",   // file holding the release notes (Markdown)
	EnvPrefix + "BRANCH",       // branch being released
	EnvPrefix + "DRY_RUN",      // "true" for nerv ship --dry-run, else "false"
}

// Vars are the values of the NERV_RELEASE_* variables for one project's
// release, supplied by the caller (nerv ship). The stage and the repository
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

// values returns the variables' values in EnvVars order.
func (v Vars) values(repoRoot, stage string) []string {
	projectPath := v.ProjectPath
	if projectPath == "" {
		projectPath = "."
	}
	return []string{
		v.Project, projectPath, repoRoot, stage, v.Version, v.Channel, v.Tag,
		v.GitHead, v.LastVersion, v.NotesFile, v.Branch, strconv.FormatBool(v.DryRun),
	}
}

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
	for i, value := range vars.values(r.RepoRoot, stage) {
		env = append(env, EnvVars[i]+"="+value)
	}
	return env
}
