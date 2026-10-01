package wizard

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/herdr"
)

// exampleRepo and exampleSlug are the placeholder {repo}/{slug} values
// substituted into every rendered git.worktree_pattern menu example.
const (
	exampleRepo = "my-repo"
	exampleSlug = "feature-tw-123-add-button"
)

// worktreePatternOption is one resolved row of the git.worktree_pattern
// menu (the "default" and, when detected, "herdr" rows): label is its
// menu name, pattern is the value choosing it writes.
type worktreePatternOption struct {
	label   string
	pattern string
}

// worktreePatternField replaces the free-text "Worktree path pattern"
// prompt with a numbered default/herdr/custom menu: the herdr row only
// appears when herdr.Detect reports Installed, in which case it stores
// "<herdr worktrees.directory>/{repo}/{slug}". Choosing "custom" falls
// through to the original free-text ask, pre-filled with current. A blank
// answer keeps current, matching every other section field. An invalid
// answer re-prompts like every other menuChoice, and ErrInputClosed
// propagates unchanged on genuine EOF.
func worktreePatternField(deps Deps, s *session, out io.Writer, current string) (string, error) {
	getenv := deps.Getenv
	if getenv == nil {
		// Optional dependency: herdr detection on non-Windows goos never
		// calls getenv, and on Windows this just degrades to an empty
		// %APPDATA%, which herdr.Detect already handles as "not found".
		getenv = func(string) string { return "" }
	}
	detection := herdr.Detect(deps.LookPath, getenv, deps.Home, os.ReadFile, runtime.GOOS)

	defaultPattern, _ := config.DefaultValue("git.worktree_pattern")
	options := []worktreePatternOption{{label: "default", pattern: defaultPattern}}
	if detection.Installed {
		options = append(options, worktreePatternOption{label: "herdr", pattern: herdr.Pattern(detection.WorktreesDir)})
	}

	fmt.Fprintf(out, "Worktree path pattern [current: %s]\n", current)
	allowed := make([]string, 0, len(options)+1)
	for i, opt := range options {
		fmt.Fprintf(out, "  %d) %-8s%-30s e.g. %s\n", i+1, opt.label, opt.pattern, renderWorktreeExample(opt.pattern))
		allowed = append(allowed, strconv.Itoa(i+1))
	}
	customChoice := strconv.Itoa(len(options) + 1)
	fmt.Fprintf(out, "  %s) custom   enter a pattern using {repo}, {slug} (also {branch}, {prefix}, {id})\n", customChoice)
	allowed = append(allowed, customChoice)

	choice, chosen, err := s.menuChoice(fmt.Sprintf("Choice [1-%s, blank keeps current]:", customChoice), allowed)
	if err != nil {
		return "", err
	}
	if !chosen {
		return current, nil
	}
	if choice == customChoice {
		return s.ask("Worktree path pattern", current)
	}
	idx, _ := strconv.Atoi(choice)
	return options[idx-1].pattern, nil
}

// renderWorktreeExample substitutes the {repo}/{slug} placeholders into
// pattern and, for a relative pattern, prefixes "<repo-root>/" so the
// rendered example reads as a full path.
func renderWorktreeExample(pattern string) string {
	substituted := strings.NewReplacer("{repo}", exampleRepo, "{slug}", exampleSlug).Replace(pattern)
	if isRelativeWorktreePattern(pattern) {
		return "<repo-root>/" + substituted
	}
	return substituted
}

// isRelativeWorktreePattern reports whether pattern is a relative path:
// not rooted at "/" or "\", not "~"-relative, and not a Windows drive
// letter such as "D:".
func isRelativeWorktreePattern(pattern string) bool {
	if pattern == "" {
		return true
	}
	if strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "\\") || strings.HasPrefix(pattern, "~") {
		return false
	}
	if len(pattern) >= 2 && pattern[1] == ':' {
		return false
	}
	return true
}
