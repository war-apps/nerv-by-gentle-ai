package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-gentle-ai/internal/wizard"
)

const configureUsage = `Usage: nerv configure [flags]

With no mode flag, runs the interactive setup wizard (when stdin is a
terminal, or when --answers is given):
  --answers <file>          Drive the wizard from a file, one answer per
                             line (blank line = keep the current value)
                             instead of prompting
  --skip-skills             Skip the required-skills install offer
  --skip-models             Skip the per-role models editor
  --skip-repos              Skip the repository init loop
  --skip-commands           Skip the Teamwork /task:* commands install
  --no-refresh              Skip the closing apply-models/cache-refresh step

Otherwise, exactly one mode flag is required:
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
	answersFile := fs.String("answers", "", "")
	skipSkills := fs.Bool("skip-skills", false, "")
	skipModels := fs.Bool("skip-models", false, "")
	skipRepos := fs.Bool("skip-repos", false, "")
	skipCommands := fs.Bool("skip-commands", false, "")
	noRefresh := fs.Bool("no-refresh", false, "")

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

	home, err := configure.ResolveHome(opts.Home)
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
		wizOpts := wizard.Options{
			Paths:        paths,
			SkipSkills:   *skipSkills,
			SkipModels:   *skipModels,
			SkipRepos:    *skipRepos,
			SkipCommands: *skipCommands,
			NoRefresh:    *noRefresh,
		}
		return runConfigureWizard(stdout, deps, opts, wizOpts, *answersFile)
	}
}

// runConfigureWizard resolves the wizard's input source — --answers when
// given, otherwise the real stdin when it is a terminal — and runs it, or
// refuses when neither applies (a non-interactive invocation with no mode
// flag and no --answers). Mirrors configure.ps1's own -AnswersFile-or-
// Read-Host duality, adapted for a Go process that has no notion of "a
// PowerShell host with no console" beyond stdin's own terminal-ness.
func runConfigureWizard(stdout io.Writer, deps configure.Deps, opts options, wizOpts wizard.Options, answersPath string) int {
	var in io.Reader
	switch {
	case answersPath != "":
		reader, err := openAnswersFile(answersPath)
		if err != nil {
			fmt.Fprintf(stdout, "nerv: %v\n", err)
			return 2
		}
		in = reader
	case stdinIsTerminal():
		in = opts.Stdin
	default:
		fmt.Fprintln(stdout, "nerv configure: stdin is not a terminal; use --print, --set, --set-model, --init-repo, --install-commands, or --answers <file>.")
		return 1
	}

	_, err := wizard.Run(deps, in, stdout, wizOpts)
	if err != nil {
		fmt.Fprintf(stdout, "nerv: %v\n", err)
		if errors.Is(err, wizard.ErrInputClosed) {
			return 1
		}
		var refusal *configure.RefusalError
		if errors.As(err, &refusal) {
			return 1
		}
		return 2
	}
	return 0
}

// openAnswersFile reads path's whole content up front (small text files, one
// answer per line). A missing file is treated as an empty answers file —
// never an error — mirroring configure.ps1's own
// `if (Test-Path $AnswersFile) { ... } else { $script:answerLines = @() }`.
func openAnswersFile(path string) (io.Reader, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return strings.NewReader(""), nil
		}
		return nil, err
	}
	return bytes.NewReader(data), nil
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
