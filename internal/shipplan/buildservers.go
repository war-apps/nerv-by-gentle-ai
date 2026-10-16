package shipplan

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// DisableBuildServers makes dotnet restore, build, test and pack run without
// the MSBuild and compiler servers. It exists from .NET SDK 7.0.100
// (MinBuildServersSDK) and dotnet format rejects it (SDK 10: "unrecognized
// command or argument"), so it is never passed to format, and a
// BuildServersProbe removes it when the SDK is older or undetected.
const DisableBuildServers = "--disable-build-servers"

// MinBuildServersSDK is the first .NET SDK that accepts DisableBuildServers.
const MinBuildServersSDK = "7.0.100"

// DotnetBuildServerEnv is the environment of the dotnet preset's steps,
// whatever the SDK: no MSBuild node reuse, no shared compilation
// (VBCSCompiler), no MSBuild server. On shared hosts (the team's T2) reused
// servers made dotnet die mid-release with "Internal CLR error
// (0x80131506)". internal/pipeline starts its commands from it too and
// applies pipeline.dotnet.env over it. Stage overrides (stages:) never get
// it. A fresh map on every call.
func DotnetBuildServerEnv() map[string]string {
	return map[string]string{
		"MSBUILDDISABLENODEREUSE":       "1",
		"UseSharedCompilation":          "false",
		"DOTNET_CLI_USE_MSBUILD_SERVER": "0",
	}
}

// sdkVersion is what dotnet --version prints: major.minor.patch with an
// optional prerelease and build metadata.
var sdkVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// SupportsDisableBuildServers reports whether out, the standard output of
// dotnet --version (its last non-empty line: a first run may print a
// welcome banner before it), is an SDK at least MinBuildServersSDK. A
// prerelease of 7.0.100 orders before it (semver), so it does not count.
// Output that is not a version is an error.
func SupportsDisableBuildServers(out string) (bool, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	m := sdkVersion.FindStringSubmatch(last)
	if m == nil {
		return false, fmt.Errorf("dotnet --version printed %q, not an SDK version", last)
	}
	v := make([]int, 3)
	for i := range v {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return false, fmt.Errorf("dotnet --version printed %q, not an SDK version", last)
		}
		v[i] = n
	}
	if c := slices.Compare(v, []int{7, 0, 100}); c != 0 {
		return c > 0, nil
	}
	return m[4] == "", nil
}

// capturer runs a step and returns its standard output; *shiprun.Runner is
// one.
type capturer interface {
	Capture(ctx context.Context, step shiprun.Step, vars shiprun.Vars) (string, error)
}

// probeTimeout bounds dotnet --version.
const probeTimeout = "2m"

// BuildServersProbe decides, for the dotnet commands of one run, whether
// DisableBuildServers may stay on their command line: it runs
// dotnet --version once per working directory (the SDK there follows its
// global.json), through the run's step runner, and caches the answer.
type BuildServersProbe struct {
	run  StepRunner
	seen map[string]bool
}

// NewBuildServersProbe returns a probe running dotnet --version through r,
// which must also capture output (*shiprun.Runner does); one that cannot
// counts as a failed detection.
func NewBuildServersProbe(r StepRunner) *BuildServersProbe {
	return &BuildServersProbe{run: r, seen: map[string]bool{}}
}

// Argv returns the command line to run for step. A dotnet command carrying
// DisableBuildServers keeps it only when the SDK of its working directory
// is at least MinBuildServersSDK; otherwise, or when the SDK cannot be
// detected, the flag is removed (the step's environment still applies).
// note is non-empty the first time detection fails in a directory, saying
// why and that the flag was left out. Other commands are returned as they
// are, without probing.
func (p *BuildServersProbe) Argv(ctx context.Context, step shiprun.Step, vars shiprun.Vars) (argv []string, note string) {
	argv = step.Exec
	if len(argv) == 0 || argv[0] != "dotnet" || !slices.Contains(argv, DisableBuildServers) {
		return argv, ""
	}
	dir := filepath.Clean(step.Command.Workdir)
	ok, known := p.seen[dir]
	if !known {
		var err error
		if ok, err = p.detect(ctx, step, vars); err != nil {
			note = fmt.Sprintf("could not detect the .NET SDK version (%v): running without %s (the build-server environment variables still apply)", err, DisableBuildServers)
		}
		p.seen[dir] = ok
	}
	if ok {
		return argv, note
	}
	return slices.DeleteFunc(slices.Clone(argv), func(a string) bool { return a == DisableBuildServers }), note
}

func (p *BuildServersProbe) detect(ctx context.Context, step shiprun.Step, vars shiprun.Vars) (bool, error) {
	c, ok := p.run.(capturer)
	if !ok {
		return false, errors.New("the step runner cannot capture output")
	}
	probe := shiprun.Step{
		Name: step.Name, Stage: step.Stage,
		Command: shipconfig.Command{Env: step.Command.Env, Workdir: step.Command.Workdir, Timeout: probeTimeout},
		Exec:    []string{"dotnet", "--version"},
	}
	out, err := c.Capture(ctx, probe, vars)
	if err != nil {
		return false, err
	}
	return SupportsDisableBuildServers(out)
}
