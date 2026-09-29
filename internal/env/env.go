// Package env is the sole seam onto the real machine — running commands,
// resolving the home directory, and looking up executables on PATH — so
// every other package can be tested against a fake instead of the real
// environment. This is the lesson of the 2026-09-28 installer incident:
// no test outside this package's own tests touches a real process or the
// real home directory.
package env

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
)

// Runner runs an external command and reports its captured output and
// exit code. A non-nil err means the command could not be launched at all
// (executable not found, permission denied, ...); a non-zero exitCode
// with a nil err means the command ran and exited unsuccessfully.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, exitCode int, err error)
}

// ExecRunner is the real Runner, backed by os/exec. It is the only type in
// this package (and, by the rg check in the go-cli feature document, in
// internal/gentleai, internal/models, or internal/skills) that imports
// os/exec.
type ExecRunner struct{}

// Run implements Runner by launching name via os/exec, capturing stdout
// and stderr separately, and translating a non-zero exit into (exitCode,
// nil error) rather than a Go error — mirroring the fact that an external
// command's non-zero exit does not throw in PowerShell either, only a
// failed launch does.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	stdout, stderr = outBuf.String(), errBuf.String()

	if runErr == nil {
		return stdout, stderr, 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return stdout, stderr, exitErr.ExitCode(), nil
	}
	return stdout, stderr, -1, runErr
}

// HomeDir resolves the user's home directory: $HOME first, then
// $USERPROFILE (an empty value counts as unset for either, matching
// PowerShell's own truthiness check). Mirrors Get-NervHomeDir.
func HomeDir() (string, error) {
	if h := os.Getenv("HOME"); h != "" {
		return h, nil
	}
	if h := os.Getenv("USERPROFILE"); h != "" {
		return h, nil
	}
	return "", errors.New("neither HOME nor USERPROFILE is set")
}

// LookPath reports the resolved path of name on PATH, or an error when it
// cannot be found. Thin wrapper kept here so callers depend on this
// package's seam instead of os/exec directly.
func LookPath(name string) (string, error) {
	return exec.LookPath(name)
}
