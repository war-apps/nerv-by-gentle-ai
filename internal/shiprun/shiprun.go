// Package shiprun runs the scripts of `nerv ship`: one stage override or
// hook command (run, shell, env, workdir, timeout) of the release: section,
// for one project.
//
// A command runs under bash, pwsh or node (the shell rules above
// scriptFile and shellArgs in shell.go give the exact command lines), in
// the repository root or its workdir (both kept inside the repository once
// symlinks are resolved), with the
// inherited environment, the command's env and the NERV_RELEASE_*
// variables (see EnvVars). Its stdout and stderr share one pipe and are
// streamed live to the caller's writer, each line prefixed with
// "[<project>:<step>] "; a failure is a *StepError naming the project, the
// step, the shell and the exit code, signal or timeout, with the last lines
// of output. Processes run through internal/proc: in their own session or
// process group, without a terminal, and a cancelled or timed-out context
// kills the whole process tree.
package shiprun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// execShell is StepError.Shell for a Step run from its Exec argument list.
const execShell = "exec"

// maxTailLines is how many of the last output lines a StepError carries.
const maxTailLines = 20

// maxLineBytes bounds a buffered output line: a longer line without a
// newline is written out in pieces of this size.
const maxLineBytes = 64 << 10

var (
	// ErrInterpreterNotFound is wrapped by the error of a command whose
	// shell (bash, pwsh or node) cannot be found.
	ErrInterpreterNotFound = errors.New("interpreter not found")
	// ErrTimeout is wrapped by the error of a command killed by its timeout.
	ErrTimeout = errors.New("timed out")
	// ErrOutsideRepository is wrapped by the error of a command whose
	// working directory or script file is, once symlinks are resolved,
	// outside the repository.
	ErrOutsideRepository = errors.New("outside the repository")
)

// Step is one command to run: a stage override, one hook command, or a
// built-in preset command.
type Step struct {
	// Name is the stage (test), hook key (before_test, prepare) or preset
	// step (dotnet:build) the command belongs to; it is the output prefix.
	Name string
	// Stage is NERV_RELEASE_STAGE; empty means Name.
	Stage   string
	Command shipconfig.Command
	// Exec, when set, is run as an argument list without any shell:
	// Exec[0] is looked up on PATH and the rest are passed as they are, so
	// nothing is quoted or expanded on any OS. Command.Run and Command.Shell
	// are then ignored; its Env, Workdir and Timeout still apply. Failures
	// name the program only, never its arguments. On Windows a program
	// found as a .cmd or .bat shim (npm, pnpm and yarn are) runs through
	// cmd.exe, which interprets & | < > ^ % in its arguments: callers must
	// keep such characters out of any configured value they pass (the
	// shipplan presets refuse them).
	Exec []string
}

// stage is the step's NERV_RELEASE_STAGE value.
func (s Step) stage() string {
	if s.Stage != "" {
		return s.Stage
	}
	return s.Name
}

// Runner runs steps for the repository at RepoRoot.
type Runner struct {
	// RepoRoot is the repository's absolute root directory.
	RepoRoot string
	// Output receives the prefixed output lines; nil discards them.
	Output io.Writer
	// Proc runs the process; nil means proc.ExecRunner.
	Proc proc.Runner
	// LookPath finds an interpreter; nil means exec.LookPath.
	LookPath func(string) (string, error)
	// Environ is the inherited environment; nil means os.Environ.
	Environ func() []string

	// Test seams for the interpreter lookup.
	goos       string
	fileExists func(string) bool
	getenv     func(string) string
}

// Run runs step for the project in vars and returns a *StepError when the
// command cannot start or does not succeed.
func (r *Runner) Run(ctx context.Context, step Step, vars Vars) error {
	c := step.Command
	shell := c.Shell
	if shell == "" {
		shell = shipconfig.DefaultShell
	}
	if len(step.Exec) > 0 {
		shell = execShell
	}
	fail := func(script, reason string, err error) error {
		return &StepError{Project: vars.Project, Step: step.Name, Shell: shell, Script: script, ExitCode: -1, Reason: reason, Err: err}
	}

	workdir := c.Workdir
	if workdir == "" {
		workdir = "."
	}
	dir := filepath.Join(r.RepoRoot, filepath.FromSlash(workdir))
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fail("", fmt.Sprintf("working directory %q is not a directory of the repository", workdir), err)
	}
	// shipconfig rejects escaping workdirs as written; this also catches a
	// hand-built Command and a symlinked directory pointing outside.
	if err := insideRepository(r.RepoRoot, dir); err != nil {
		err = fmt.Errorf("working directory %q %w", workdir, err)
		return fail("", err.Error(), err)
	}
	var interp, script string
	var args []string
	if len(step.Exec) > 0 {
		// The program stands where a script file would in errors.
		script = step.Exec[0]
		p, err := r.lookPath(script)
		if err != nil {
			err = &interpreterError{script + " not found on PATH: install " + script}
			return fail(script, err.Error(), err)
		}
		interp, args = p, step.Exec[1:]
	} else {
		var err error
		if interp, err = r.interpreter(shell); err != nil {
			return fail("", err.Error(), err)
		}
		if script, err = scriptFile(r.RepoRoot, shell, c.Run); err != nil {
			return fail("", err.Error(), err)
		}
		args = shellArgs(shell, c.Run, script, r.RepoRoot, dir)
	}

	runCtx := ctx
	timeout := c.TimeoutDuration()
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	out := r.Output
	if out == nil {
		out = io.Discard
	}
	pw := newPrefixWriter(out, "["+vars.Project+":"+step.Name+"] ", maxLineBytes)
	res, runErr := r.proc().Run(runCtx, proc.Command{
		Name:   interp,
		Args:   args,
		Dir:    dir,
		Env:    r.environment(c.Env, vars, step.stage()),
		Stdout: pw,
		Stderr: pw,
	})
	pw.Flush()
	if runErr == nil {
		if pw.err != nil {
			return fail(script, "writing its output: "+pw.err.Error(), pw.err)
		}
		return nil
	}

	se := &StepError{Project: vars.Project, Step: step.Name, Shell: shell, Script: script, ExitCode: -1, Output: lastLines(ansiRe.ReplaceAllString(string(res.Stdout), ""), maxTailLines), Err: runErr}
	var coder interface{ ExitCode() int }
	switch {
	case ctx.Err() != nil:
		se.Reason, se.Err = "interrupted", ctx.Err()
	case timeout > 0 && errors.Is(runCtx.Err(), context.DeadlineExceeded):
		se.Reason, se.Err = "timed out after "+timeout.String(), ErrTimeout
	case errors.As(runErr, &coder) && coder.ExitCode() >= 0:
		se.ExitCode = coder.ExitCode()
		se.Reason = fmt.Sprintf("exit code %d", se.ExitCode)
	case errors.As(runErr, &coder):
		se.Reason = "killed (" + runErr.Error() + ")"
	default:
		se.Reason = "could not run: " + runErr.Error()
	}
	return se
}

func (r *Runner) proc() proc.Runner {
	if r.Proc == nil {
		return proc.ExecRunner{}
	}
	return r.Proc
}

// StepError is a step that could not run or did not succeed. Its message
// never contains environment values.
type StepError struct {
	Project string
	Step    string
	Shell   string
	// Script is the script file run (repository-relative, as written), the
	// program of an Exec step (Shell is then "exec"), or empty for an
	// inline command.
	Script string
	// ExitCode is the process's exit code, -1 when it has none (not started,
	// killed by a signal, timed out, interrupted).
	ExitCode int
	// Reason says what happened: "exit code 3", "timed out after 10m", ...
	Reason string
	// Output is the last lines of the command's output.
	Output string
	// Err is the cause: ErrTimeout, ErrInterpreterNotFound, the context's
	// error when interrupted, or the process error.
	Err error
}

func (e *StepError) Error() string {
	what := e.Shell
	if e.Script != "" {
		what += " " + e.Script
	}
	msg := fmt.Sprintf("project %q, step %q (%s): %s", e.Project, e.Step, what, e.Reason)
	if e.Output != "" {
		msg += "\nlast output:\n" + e.Output
	}
	return msg
}

func (e *StepError) Unwrap() error { return e.Err }

// ansiRe matches a terminal escape sequence (colours, cursor moves): the
// live output keeps them, the error's output tail does not.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// lastLines returns at most n trailing lines of text, without the final
// newline.
func lastLines(text string, n int) string {
	text = strings.TrimRight(text, "\r\n")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// lookPath, environ, targetOS, exists and getEnv default to the real
// system.
func (r *Runner) lookPath(name string) (string, error) {
	if r.LookPath != nil {
		return r.LookPath(name)
	}
	return exec.LookPath(name)
}

func (r *Runner) environ() []string {
	if r.Environ != nil {
		return r.Environ()
	}
	return os.Environ()
}

func (r *Runner) targetOS() string {
	if r.goos != "" {
		return r.goos
	}
	return runtime.GOOS
}

func (r *Runner) exists(path string) bool {
	if r.fileExists != nil {
		return r.fileExists(path)
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (r *Runner) getEnv(name string) string {
	if r.getenv != nil {
		return r.getenv(name)
	}
	return os.Getenv(name)
}
