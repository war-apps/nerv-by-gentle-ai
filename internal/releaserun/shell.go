package releaserun

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Shell rules. A command's run is a script file when, trimmed, it is a
// single line naming an existing regular file relative to the repository
// root (backslashes accepted, no escaping the root) with an extension the
// shell runs as a file: any for bash, .ps1 for pwsh, .js, .mjs or .cjs for
// node. The file is passed relative to the working directory (./x or
// ../x). Anything else, including a path followed by arguments, is an
// inline command:
//
//	bash file    bash --noprofile --norc <file>
//	bash inline  bash --noprofile --norc -e -o pipefail -c <run>
//	pwsh file    pwsh -NoLogo -NoProfile -NonInteractive -File <file>
//	pwsh inline  pwsh -NoLogo -NoProfile -NonInteractive -Command <pwshPrelude><run><pwshEpilogue>
//	node file    node <file>
//	node inline  node -e <run>
//
// A script file runs as written (its own `set -e`, $ErrorActionPreference
// or exit codes decide); inline commands stop at the first failure, like a
// CI step: bash with -e and pipefail, pwsh with the prelude below. An
// inline pwsh command's exit code follows how the script ended: 0 when it
// ran to its end, 1 on an uncaught error (an unhandled native failure
// included), N on an explicit `exit N`.
//
// Inline pwsh commands need PowerShell 7.3 or newer; an older pwsh exits
// pwshTooOldExit before running the command.
//
// A script file whose real location (symlinks resolved) is outside the
// repository is refused (ErrOutsideRepository).

// pwshTooOldExit is the exit code of an inline pwsh command refused because
// the PowerShell running it is older than 7.3.
const pwshTooOldExit = 3

// pwshPrelude makes an inline pwsh command stop at the first error: cmdlet
// errors become terminating, and so does a native command's non-zero exit.
// A terminating error exits 1. The native-command part needs PowerShell 7.3
// ($PSNativeCommandUseErrorActionPreference); without it a failing native
// command as the last statement would reach the `exit 0` epilogue and look
// like a success, so an older pwsh is refused first.
var pwshPrelude = buildPwshPrelude(7, 3)

// buildPwshPrelude returns the prelude requiring PowerShell major.minor or
// newer.
func buildPwshPrelude(major, minor int) string {
	return fmt.Sprintf("$nervPs = $PSVersionTable.PSVersion; "+
		"if ($nervPs.Major -lt %[1]d -or ($nervPs.Major -eq %[1]d -and $nervPs.Minor -lt %[2]d)) { "+
		"[Console]::Error.WriteLine(\"nerv needs PowerShell %[1]d.%[2]d or newer for inline commands (found $nervPs)\"); exit %[3]d }\n"+
		"$ErrorActionPreference = 'Stop'; $PSNativeCommandUseErrorActionPreference = $true\n",
		major, minor, pwshTooOldExit)
}

const (
	// pwshEpilogue makes a command that ran to its end exit 0. Without it
	// pwsh -Command exits with $LASTEXITCODE, which still holds a native
	// failure the script handled (try/catch, or checked and continued).
	// An unhandled native failure already stopped the script through the
	// prelude (exit 1), and an explicit `exit N` never reaches this line.
	pwshEpilogue = "\nexit 0"
)

// scriptFile returns the script file run names, as a clean
// repository-relative path with forward slashes, or "" for an inline
// command (see the shell rules). The returned path is the one checked and
// the one executed. A name that is a script file for the shell but whose
// real location (symlinks resolved) is outside the repository is an error
// wrapping ErrOutsideRepository, never an inline command.
func scriptFile(repoRoot, shell, run string) (string, error) {
	name := strings.TrimSpace(run)
	if name == "" || strings.ContainsAny(name, "\r\n") {
		return "", nil
	}
	clean := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if !filepath.IsLocal(filepath.FromSlash(clean)) {
		return "", nil
	}
	ext := strings.ToLower(path.Ext(clean))
	switch shell {
	case "pwsh":
		if ext != ".ps1" {
			return "", nil
		}
	case "node":
		if ext != ".js" && ext != ".mjs" && ext != ".cjs" {
			return "", nil
		}
	}
	file := filepath.Join(repoRoot, filepath.FromSlash(clean))
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		return "", nil
	}
	if err := insideRepository(repoRoot, file); err != nil {
		return "", fmt.Errorf("script file %q: %w", clean, err)
	}
	return clean, nil
}

// insideRepository returns an error wrapping ErrOutsideRepository unless
// target, with every symlink resolved, is the repository root or below it.
func insideRepository(repoRoot, target string) error {
	realRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(realRoot, real)
	if err != nil || (rel != "." && !filepath.IsLocal(rel)) {
		return fmt.Errorf("resolves to %s, %w", real, ErrOutsideRepository)
	}
	return nil
}

// shellArgs returns the interpreter's arguments for run, given the clean
// script file it names ("" for inline, see scriptFile), the repository
// root and the working directory.
func shellArgs(shell, run, script, repoRoot, dir string) []string {
	file := ""
	if script != "" {
		file = relativeTo(dir, filepath.Join(repoRoot, filepath.FromSlash(script)))
	}
	switch shell {
	case "pwsh":
		if file != "" {
			return []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-File", file}
		}
		return []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", pwshPrelude + run + pwshEpilogue}
	case "node":
		if file != "" {
			return []string{file}
		}
		return []string{"-e", run}
	default:
		if file != "" {
			// Git Bash on Windows reads forward slashes.
			return []string{"--noprofile", "--norc", filepath.ToSlash(file)}
		}
		return []string{"--noprofile", "--norc", "-e", "-o", "pipefail", "-c", run}
	}
}

// relativeTo returns target relative to dir, starting with ./ or ../ so
// that no interpreter reads it as an option or looks it up in PATH.
func relativeTo(dir, target string) string {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return target
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel
	}
	return "." + string(filepath.Separator) + rel
}

// interpreterError is a missing interpreter; it matches
// ErrInterpreterNotFound.
type interpreterError struct{ msg string }

func (e *interpreterError) Error() string        { return e.msg }
func (e *interpreterError) Is(target error) bool { return target == ErrInterpreterNotFound }

// interpreter returns the executable of shell. On Windows, bash is Git
// Bash (see windowsBash).
func (r *Runner) interpreter(shell string) (string, error) {
	if shell == "bash" && r.targetOS() == "windows" {
		return r.windowsBash()
	}
	if p, err := r.lookPath(shell); err == nil {
		return p, nil
	}
	switch shell {
	case "pwsh":
		return "", &interpreterError{"pwsh not found on PATH: install PowerShell 7 (https://aka.ms/powershell); Windows PowerShell 5.1 (powershell.exe) is not used"}
	case "node":
		return "", &interpreterError{"node not found on PATH: install Node.js (https://nodejs.org)"}
	default:
		return "", &interpreterError{shell + " not found on PATH: install " + shell}
	}
}

// windowsBash finds Git Bash: bash on PATH unless it is WSL's launcher
// (System32\bash.exe or its WindowsApps alias, which would run the command
// inside a Linux distribution), then bin\bash.exe in the Git for Windows
// installation that holds git.exe on PATH, then the default installation
// directories.
func (r *Runner) windowsBash() (string, error) {
	if p, err := r.lookPath("bash"); err == nil && !isWSLBash(p) {
		return p, nil
	}
	if g, err := r.lookPath("git"); err == nil {
		// git.exe lives in <Git>\cmd, <Git>\bin or <Git>\mingw64\bin.
		dir := filepath.Dir(g)
		for range 3 {
			if c := filepath.Join(dir, "bin", "bash.exe"); r.exists(c) {
				return c, nil
			}
			dir = filepath.Dir(dir)
		}
	}
	var bases []string
	if pf := r.getEnv("ProgramFiles"); pf != "" {
		bases = append(bases, pf)
	}
	if local := r.getEnv("LocalAppData"); local != "" {
		bases = append(bases, filepath.Join(local, "Programs"))
	}
	for _, base := range bases {
		if c := filepath.Join(base, "Git", "bin", "bash.exe"); r.exists(c) {
			return c, nil
		}
	}
	return "", &interpreterError{`bash not found: install Git for Windows; NERV runs Git Bash's bash.exe (on PATH, next to git.exe, or in %ProgramFiles%\Git\bin), never WSL's bash.exe`}
}

// isWSLBash reports whether p is WSL's bash launcher.
func isWSLBash(p string) bool {
	lower := strings.ToLower(filepath.ToSlash(p))
	return strings.Contains(lower, "/system32/") || strings.Contains(lower, "/windowsapps/")
}
