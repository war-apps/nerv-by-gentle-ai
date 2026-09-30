// Package refusal defines the one error type every nerv use case
// (configure, install, wizard, release) wraps a rejected request in — an
// unknown key, an invalid value, a malformed argument, a missing path, a
// non-git directory, a tag that already exists — as opposed to an
// unexpected environment failure (I/O, permissions, a missing home
// directory, a git launch failure). cmd/nerv maps the distinction to a
// process exit code: 1 for a refusal, 2 for anything else.
package refusal

import "errors"

// Error marks a rejected request. Use cases construct one directly
// (&refusal.Error{Err: ...}); callers test for it with Is.
type Error struct {
	Err error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Is reports whether err is, or wraps, a *Error.
func Is(err error) bool {
	var e *Error
	return errors.As(err, &e)
}
