// Package herdr detects a local herdr (https://herdr.dev-style worktree
// manager) installation and resolves the worktree root it is configured
// to use, so NERV can offer it as a "git.worktree_pattern" menu option and
// reuse it instead of asking the user to retype a directory herdr already
// knows about.
//
// This package never touches the real filesystem or PATH itself: every
// effect (LookPath, Getenv, ReadFile) is injected, so callers can fake
// them in tests exactly like every other internal package's seam.
package herdr

import (
	"bufio"
	"bytes"
	"strings"
)

// executableName is the herdr binary name looked up on PATH.
const executableName = "herdr"

// defaultWorktreesDirPattern is herdr's own built-in default for
// [worktrees] directory, used when herdr is installed but its config file
// is missing, unreadable, or does not set the key.
const defaultWorktreesDirPattern = "~/.herdr/worktrees"

// Detection reports whether herdr was found on PATH and, when it was, the
// worktree root it is configured to use (already "~"-expanded).
type Detection struct {
	Installed    bool
	WorktreesDir string
}

// Detect probes for a local herdr installation: not on PATH means
// Installed is false and WorktreesDir stays empty. Installed means
// WorktreesDir resolves from herdr's own config.toml (`[worktrees]
// directory = "..."`), falling back to herdr's documented default
// (~/.herdr/worktrees) when the file is missing, unreadable, or the key
// is set outside the [worktrees] section.
//
// goos selects the config file's OS-specific location the same way
// runtime.GOOS would in production (callers pass runtime.GOOS there;
// tests pass a fixed value to exercise both branches without build
// tags): "windows" resolves to "<APPDATA>\herdr\config.toml", every other
// value to "<home>/.config/herdr/config.toml".
func Detect(lookPath func(string) (string, error), getenv func(string) string, home string, readFile func(string) ([]byte, error), goos string) Detection {
	if _, err := lookPath(executableName); err != nil {
		return Detection{}
	}

	dir := expandHome(defaultWorktreesDirPattern, home)
	if data, err := readFile(configPath(getenv, home, goos)); err == nil {
		if value, ok := parseWorktreesDirectory(data); ok {
			dir = expandHome(value, home)
		}
	}
	return Detection{Installed: true, WorktreesDir: dir}
}

// Pattern builds the git.worktree_pattern value for a herdr worktrees
// directory: "<worktreesDir>/{repo}/{slug}", joined with whichever path
// separator worktreesDir already uses (backslash for a Windows-style
// directory, forward slash otherwise) instead of normalizing it through
// filepath.Join — herdr's own directory is shown to the user exactly as
// herdr reports it.
func Pattern(worktreesDir string) string {
	sep := separatorOf(worktreesDir)
	trimmed := strings.TrimRight(worktreesDir, `/\`)
	return trimmed + sep + "{repo}" + sep + "{slug}"
}

// separatorOf reports the path separator worktreesDir already uses: a
// backslash if present, else a forward slash — including when
// worktreesDir has no separator at all, since a forward slash is the safe
// cross-platform default for a bare name.
func separatorOf(worktreesDir string) string {
	if strings.ContainsRune(worktreesDir, '\\') {
		return "\\"
	}
	return "/"
}

// expandHome replaces a leading "~" with home, preserving whatever
// separator follows it. Values without a leading "~" are returned
// unchanged.
func expandHome(value, home string) string {
	if strings.HasPrefix(value, "~") {
		return home + value[1:]
	}
	return value
}

// configPath resolves herdr's own config.toml location for goos:
// "%APPDATA%\herdr\config.toml" on Windows, "~/.config/herdr/config.toml"
// (already expanded against home) elsewhere.
func configPath(getenv func(string) string, home, goos string) string {
	if goos == "windows" {
		appData := strings.TrimRight(getenv("APPDATA"), `\/`)
		return appData + `\herdr\config.toml`
	}
	return strings.TrimRight(home, "/") + "/.config/herdr/config.toml"
}

// parseWorktreesDirectory does a minimal TOML section/key scan of data,
// tracking the current `[section]` and returning the `directory` value
// found while inside `[worktrees]`. A `directory` key inside any other
// section is ignored. ok is false when the file has no such key.
func parseWorktreesDirectory(data []byte) (value string, ok bool) {
	section := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != "worktrees" {
			continue
		}
		key, raw, found := splitAssignment(line)
		if !found || key != "directory" {
			continue
		}
		if parsed, parsedOK := parseTOMLString(raw); parsedOK {
			return parsed, true
		}
	}
	return "", false
}

// splitAssignment splits a "key = value" line on its first "=".
func splitAssignment(line string) (key, value string, ok bool) {
	idx := strings.Index(line, "=")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

// parseTOMLString interprets raw (the right-hand side of a "key = value"
// line) as a minimal TOML string: a double-quoted string (with "\\"
// unescaped to "\"), a single-quoted literal string (no escaping), or an
// unquoted bare value with any trailing "# comment" stripped.
func parseTOMLString(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	switch raw[0] {
	case '"':
		return parseQuoted(raw, '"', true)
	case '\'':
		return parseQuoted(raw, '\'', false)
	default:
		if i := strings.Index(raw, "#"); i >= 0 {
			raw = raw[:i]
		}
		return strings.TrimSpace(raw), true
	}
}

// parseQuoted returns the content of a quote-delimited string starting at
// raw[0] == quote, unescaping "\\" to "\" only when unescape is true
// (double-quoted strings; single-quoted TOML literals never escape).
func parseQuoted(raw string, quote byte, unescape bool) (string, bool) {
	for i := 1; i < len(raw); i++ {
		if unescape && raw[i] == '\\' {
			i++
			continue
		}
		if raw[i] == quote {
			content := raw[1:i]
			if unescape {
				content = strings.ReplaceAll(content, `\\`, `\`)
			}
			return content, true
		}
	}
	return "", false
}
