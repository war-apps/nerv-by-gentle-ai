package semrel

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Command is one external process: git, node or npm.
type Command struct {
	// Name is the executable, looked up in PATH.
	Name string
	Args []string
	// Dir is the working directory; empty means the current one.
	Dir string
	// Env is the full environment; nil inherits the current one.
	Env []string
	// Stdin is written to the process's standard input.
	Stdin []byte
}

func (c Command) String() string { return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " ")) }

// Result is what a finished process wrote.
type Result struct {
	Stdout []byte
	Stderr []byte
}

// Runner runs external processes. It is the seam that lets the engine be
// tested without git, node or npm. Run returns the output even when the
// process fails; the error then wraps exec.ErrNotFound when the executable
// does not exist.
type Runner interface {
	Run(ctx context.Context, c Command) (Result, error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, c Command) (Result, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = c.Env
	if c.Stdin != nil {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

// run runs c and turns a failure into an error carrying the end of stderr.
func (e *Engine) run(ctx context.Context, c Command) (Result, error) {
	res, err := e.runner().Run(ctx, c)
	if err != nil {
		return res, fmt.Errorf("%s: %w%s", c, err, stderrTail(res.Stderr))
	}
	return res, nil
}

// stderrTail returns the last lines of a process's stderr, for errors.
func stderrTail(stderr []byte) string {
	text := strings.TrimSpace(string(stderr))
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 15 {
		lines = lines[len(lines)-15:]
	}
	return "\n" + strings.Join(lines, "\n")
}
