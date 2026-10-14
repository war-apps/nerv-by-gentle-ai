package semrel

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// Command is one external process: git, node or npm.
type Command = proc.Command

// Result is what a finished process wrote.
type Result = proc.Result

// Runner runs external processes. It is the seam that lets the engine be
// tested without git, node or npm. Run returns the output even when the
// process fails; the error then wraps exec.ErrNotFound when the executable
// does not exist.
type Runner = proc.Runner

// ExecRunner is the real Runner: the shared, hardened process runner of
// internal/proc (own session or process group, whole-tree kill on cancel,
// bounded wait, no terminal). A terminal Ctrl-C reaches its commands only
// through context cancellation (signal.NotifyContext).
type ExecRunner = proc.ExecRunner

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
