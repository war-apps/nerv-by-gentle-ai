package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/configure"
)

const configureUsage = `Usage: nerv configure [flags]

Exactly one mode flag is required:
  --print                   Print the current configuration as JSON
  --set key=value           Set a managed config key (repeatable)
  --set-model role=spec     Set a role's model/effort override (repeatable);
                             spec is model[/effort], from:<phase>, or default
  --init-repo <path>        Initialize a repository for NERV
  --install-commands        Install the /task:* Teamwork slash commands

With --init-repo:
  --repo-base <branch>      Base branch override
  --repo-provider <name>    Tasks provider override (teamwork | github-projects | jira | none)
  --repo-project-id <id>    Teamwork project id override
  --repo-tasklist-id <id>   Teamwork tasklist id override

Other flags:
  --json                    Print the result as JSON (ignored by --print, which always does)
  --config <path>           Override the user-scope nerv.yaml path
  --home <dir>              Override the resolved home directory

The interactive wizard is not available yet in this build; pass exactly one
mode flag above.
`

// stringList is a repeatable string flag: each --flag value appends an
// entry instead of replacing the previous one, giving --set/--set-model
// their documented "repeatable" behavior natively (unlike configure.ps1's
// -Set/-SetModel, whose PowerShell array parameters needed the -Command
// workaround documented in tests/configure.test.ps1's case group K).
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func runConfigure(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("configure", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	printMode := fs.Bool("print", false, "")
	var setArgs stringList
	fs.Var(&setArgs, "set", "")
	var setModelArgs stringList
	fs.Var(&setModelArgs, "set-model", "")
	initRepo := fs.String("init-repo", "", "")
	repoBase := fs.String("repo-base", "", "")
	repoProvider := fs.String("repo-provider", "", "")
	repoProjectID := fs.String("repo-project-id", "", "")
	repoTasklistID := fs.String("repo-tasklist-id", "", "")
	installCommands := fs.Bool("install-commands", false, "")
	jsonOut := fs.Bool("json", false, "")
	configOverride := fs.String("config", "", "")
	homeOverride := fs.String("home", "", "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprint(stderr, configureUsage)
		return 2
	}

	modeCount := 0
	if *printMode {
		modeCount++
	}
	if len(setArgs) > 0 {
		modeCount++
	}
	if len(setModelArgs) > 0 {
		modeCount++
	}
	if *initRepo != "" {
		modeCount++
	}
	if *installCommands {
		modeCount++
	}
	if modeCount > 1 {
		fmt.Fprint(stderr, configureUsage)
		return 2
	}

	home, err := configure.ResolveHome(*homeOverride)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		return 2
	}
	paths := configure.ResolvePaths(home, *configOverride)
	deps := configure.Deps{Home: home, FS: opts.PluginFS, Runner: opts.Runner, Now: opts.Now, LookPath: opts.LookPath}

	switch {
	case *printMode:
		result, err := configure.Print(deps, paths)
		if err != nil {
			fmt.Fprintf(stdout, "nerv: %v\n", err)
			return 2
		}
		writeJSON(stdout, result)
		return 0

	case len(setArgs) > 0:
		result, err := configure.Set(deps, paths, setArgs)
		return emitMutation(stdout, result, err, *jsonOut)

	case len(setModelArgs) > 0:
		result, err := configure.SetModel(deps, paths, setModelArgs)
		return emitMutation(stdout, result, err, *jsonOut)

	case *initRepo != "":
		req := configure.InitRepoRequest{
			Path:       *initRepo,
			Base:       *repoBase,
			Provider:   *repoProvider,
			ProjectID:  *repoProjectID,
			TasklistID: *repoTasklistID,
		}
		result, err := configure.InitRepo(deps, req)
		return emitMutation(stdout, result, err, *jsonOut)

	case *installCommands:
		result, err := configure.InstallCommands(deps, paths)
		return emitMutation(stdout, result, err, *jsonOut)

	default:
		fmt.Fprintln(stdout, "nerv configure: the interactive wizard is not available yet in this build; pass --print, --set, --set-model, --init-repo, or --install-commands.")
		return 1
	}
}

func writeJSON(w io.Writer, v any) {
	encoded, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(w, "nerv: %v\n", err)
		return
	}
	fmt.Fprintf(w, "%s\n", encoded)
}

// emitMutation formats one of Set/SetModel/InitRepo/InstallCommands'
// results (or its refusal/environment error) and returns the process exit
// code: 0 on success, 1 on a *configure.RefusalError, 2 on any other error.
// Every branch writes to stdout, mirroring configure.ps1's uniform use of
// Write-Host (and ConvertTo-Json's own default stream) for every outcome —
// success, refusal, or unexpected error alike.
func emitMutation(stdout io.Writer, result configure.Result, err error, jsonOut bool) int {
	if err != nil {
		fmt.Fprintln(stdout, err.Error())
		var refusal *configure.RefusalError
		if errors.As(err, &refusal) {
			return 1
		}
		return 2
	}

	if jsonOut {
		writeJSON(stdout, result)
		return 0
	}

	if !result.Changed {
		fmt.Fprintln(stdout, "No changes.")
	} else {
		fmt.Fprintln(stdout, "Changed:")
		for _, c := range result.Changes {
			fmt.Fprintf(stdout, "  %s: %s -> %s\n", c.Key, c.From, c.To)
		}
		fmt.Fprintln(stdout, "Written:")
		for _, w := range result.Written {
			fmt.Fprintf(stdout, "  %s\n", w)
		}
	}
	for _, w := range result.Warnings {
		fmt.Fprintf(stdout, "Warning: %s\n", w)
	}
	return 0
}
