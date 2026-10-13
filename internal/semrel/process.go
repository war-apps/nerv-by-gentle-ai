package semrel

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// processWaitDelay bounds how long Run waits for a process's output pipes
// to close after the process exited or its context was cancelled: a
// grandchild that inherited them (an ssh or https helper under git fetch,
// an install script under npm) must not keep Run blocked.
const processWaitDelay = 5 * time.Second

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

// ExecRunner runs commands with os/exec. Each command runs in its own
// process group (a new process group on Windows); cancelling the context
// kills the whole group, so the helpers a command started (ssh,
// git-remote-https, credential helpers, npm's children) die with it, and
// Run returns at most processWaitDelay after the cancellation or the
// command's exit even if one of them still holds the output pipes.
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
	cmd.WaitDelay = processWaitDelay
	killGroupOnCancel(cmd)
	err := cmd.Run()
	return Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

// run runs c and turns a failure into an error carrying the end of stderr.
func (e *Engine) run(ctx context.Context, c Command) (Result, error) {
	res, err := e.runner().Run(ctx, c)
	if err != nil {
		return res, fmt.Errorf("%s: %w%s", redact(c.String()), err, stderrTail(res.Stderr))
	}
	return res, nil
}

// stderrTail returns the last lines of a process's stderr, for errors, with
// URL credentials redacted (git echoes the URL it failed to read).
func stderrTail(stderr []byte) string {
	text := strings.TrimSpace(redact(string(stderr)))
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 15 {
		lines = lines[len(lines)-15:]
	}
	return "\n" + strings.Join(lines, "\n")
}

var (
	// urlUserinfoRe matches the userinfo of a URL ("scheme://user:pass@").
	urlUserinfoRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s@'"]+@`)
	// secretQueryRe matches a URL query parameter whose name says it holds a
	// secret (token, access_token, private_token, password, secret, key, sig).
	secretQueryRe = regexp.MustCompile(`(?i)([?&][a-z0-9_.-]*(?:token|password|passwd|secret|key|sig)[a-z0-9_.-]*=)[^&\s'"#]+`)
)

// redact hides the credentials of every URL in s: the userinfo
// (https://user:token@host -> https://***@host) and secret-looking query
// values (?access_token=x -> ?access_token=***). Every command string and
// process output the engine puts in an error goes through it.
func redact(s string) string {
	s = urlUserinfoRe.ReplaceAllString(s, "${1}***@")
	return secretQueryRe.ReplaceAllString(s, "${1}***")
}
