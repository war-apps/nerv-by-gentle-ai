package shiprun

import (
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
// CI step: bash with -e and pipefail, pwsh with the prelude below.
const (
	// pwshPrelude makes an inline pwsh command stop at the first error:
	// cmdlet errors become terminating, and so does a native command's
	// non-zero exit (PowerShell 7.3 or newer). A terminating error exits 1.
	pwshPrelude = "$ErrorActionPreference = 'Stop'; $PSNativeCommandUseErrorActionPreference = $true\n"
	// pwshEpilogue passes on the exit code of the last native command,
	// which pwsh -Command would otherwise reduce to 0 or 1.
	pwshEpilogue = "\nif ((Test-Path -LiteralPath variable:\\LASTEXITCODE)) { exit $LASTEXITCODE }"
)

// scriptFile returns run, trimmed, when it names a script file the shell
// runs as a file (see the shell rules), or "" for an inline command.
func scriptFile(repoRoot, shell, run string) string {
	name := strings.TrimSpace(run)
	if name == "" || strings.ContainsAny(name, "\r\n") {
		return ""
	}
	clean := filepath.FromSlash(path.Clean(strings.ReplaceAll(name, `\`, "/")))
	if !filepath.IsLocal(clean) {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(clean))
	switch shell {
	case "pwsh":
		if ext != ".ps1" {
			return ""
		}
	case "node":
		if ext != ".js" && ext != ".mjs" && ext != ".cjs" {
			return ""
		}
	}
	info, err := os.Stat(filepath.Join(repoRoot, clean))
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return name
}

// shellArgs returns the interpreter's arguments for run, given the script
// file it names ("" for inline), the repository root and the working
// directory.
func shellArgs(shell, run, script, repoRoot, dir string) []string {
	file := ""
	if script != "" {
		abs := filepath.Join(repoRoot, filepath.FromSlash(path.Clean(strings.ReplaceAll(script, `\`, "/"))))
		file = relativeTo(dir, abs)
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
	for _, base := range []string{r.getEnv("ProgramFiles"), filepath.Join(r.getEnv("LocalAppData"), "Programs")} {
		if base == "" || base == "Programs" {
			continue
		}
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
