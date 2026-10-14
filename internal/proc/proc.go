// Package proc runs external processes for NERV's release commands: git,
// npm and node for `nerv version next`, and the stage and hook scripts of
// `nerv ship`.
//
// Every command runs in its own session and process group (Unix) or new
// process group (Windows), with standard input detached unless given, so a
// cancelled context kills the whole process tree and a helper that would
// prompt on the terminal fails instead of waiting.
package proc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"time"
)

// waitDelay bounds how long Run waits for a process's output pipes to close
// after the process exited or its context was cancelled: a grandchild that
// inherited them (an ssh or https helper under git fetch, an install script
// under npm) must not keep Run blocked. A variable only so tests can shorten
// it; tests that change it must not run in parallel.
var waitDelay = 5 * time.Second

// Command is one external process.
type Command struct {
	// Name is the executable, looked up in PATH.
	Name string
	Args []string
	// Dir is the working directory; empty means the current one.
	Dir string
	// Env is the full environment; nil inherits the current one.
	Env []string
	// Stdin is written to the process's standard input; nil means none
	// (the null device).
	Stdin []byte
	// Stdout and Stderr, when set, receive the process's output live, as it
	// is written (in whatever chunks the process writes; the caller does
	// any line buffering). A streamed output keeps only its last TailLimit
	// bytes in the Result, for error reports. When both are the same
	// writer, the process gets one pipe for both streams, so their relative
	// order is the order it wrote them in, and the combined tail is
	// Result.Stdout.
	Stdout io.Writer
	Stderr io.Writer
	// TailLimit bounds the Result of a streamed output; 0 means
	// DefaultTailLimit. An output without a writer is captured whole.
	TailLimit int
}

// DefaultTailLimit is how much of a streamed output a Result keeps.
const DefaultTailLimit = 64 << 10

func (c Command) String() string { return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " ")) }

// Result is what a finished process wrote.
type Result struct {
	Stdout []byte
	Stderr []byte
}

// Runner runs external processes. It is the seam that lets callers be
// tested without real processes. Run returns the output even when the
// process fails; the error then wraps exec.ErrNotFound when the executable
// does not exist.
type Runner interface {
	Run(ctx context.Context, c Command) (Result, error)
}

// ExecRunner runs commands with os/exec. Each command runs in its own
// session and process group on Unix (a new process group on Windows);
// cancelling the context kills the whole group, so the helpers a command
// started (ssh, git-remote-https, credential helpers, npm's children) die
// with it, and Run returns at most waitDelay after the cancellation or the
// command's exit even if one of them still holds the output pipes. A
// command that exited 0 while a descendant held the pipes is a success: its
// result is the output written until then, and the leftovers are killed (on
// Unix).
//
// Because commands run outside the terminal's foreground process group and
// without a controlling terminal, a terminal Ctrl-C does not reach them, and
// a helper that prompts on the terminal fails instead of waiting: the caller
// stops them on Ctrl-C only by cancelling the context (signal.NotifyContext).
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, c Command) (Result, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = c.Env
	if c.Stdin != nil {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}
	stdout, stderr := c.outputs()
	cmd.Stdout = stdout.w
	cmd.Stderr = stderr.w
	cmd.WaitDelay = waitDelay
	killGroupOnCancel(cmd)
	err := cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		// The command exited, but a descendant kept the output pipes open
		// past the wait delay: kill what is left of the tree. Wait reports
		// ErrWaitDelay only when the command itself succeeded, so a run
		// that was not cancelled is a success.
		killLeftovers(cmd)
		if ctx.Err() == nil && cmd.ProcessState != nil && cmd.ProcessState.Success() {
			err = nil
		}
	}
	return Result{Stdout: stdout.bytes(), Stderr: stderr.bytes()}, err
}

// output is where one stream of a process goes, and what the Result keeps.
type output struct {
	w     io.Writer
	bytes func() []byte
}

// outputs builds the process's stdout and stderr destinations: a whole
// capture without a writer, or the writer plus a bounded tail. The same
// writer for both shares one destination value, which os/exec turns into
// one pipe.
func (c Command) outputs() (stdout, stderr output) {
	limit := c.TailLimit
	if limit <= 0 {
		limit = DefaultTailLimit
	}
	build := func(w io.Writer) output {
		if w == nil {
			var b bytes.Buffer
			return output{w: &b, bytes: b.Bytes}
		}
		tail := newTailBuffer(limit)
		// The tail first: it never fails, so it holds the output up to
		// a failing caller writer too.
		return output{w: io.MultiWriter(tail, w), bytes: tail.Bytes}
	}
	stdout = build(c.Stdout)
	if c.Stderr != nil && sameWriter(c.Stdout, c.Stderr) {
		return stdout, output{w: stdout.w, bytes: func() []byte { return nil }}
	}
	return stdout, build(c.Stderr)
}

// sameWriter reports whether a and b are the same comparable writer.
func sameWriter(a, b io.Writer) bool {
	if a == nil || b == nil || !reflect.TypeOf(a).Comparable() || reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false
	}
	return a == b
}
