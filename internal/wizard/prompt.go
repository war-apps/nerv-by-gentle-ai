package wizard

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// maxInvalidAttempts bounds every re-prompt loop below at 3 attempts (as
// configure.ps1's Get-NervChoiceOrDefault/Read-NervChoice would keep
// looping forever on an interactive terminal): once exhausted, the prompt
// falls back to its "keep current"/"no change" answer instead of spinning
// on a scripted or exhausted reader.
const maxInvalidAttempts = 3

// session drives every prompt in one wizard run from a single shared
// line-based reader, so answers are consumed in order exactly once —
// mirroring configure.ps1's -AnswersFile index cursor
// ($script:answerIndex).
type session struct {
	in  *bufio.Scanner
	out io.Writer
}

func newSession(in io.Reader, out io.Writer) *session {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	return &session{in: scanner, out: out}
}

// readLineOr returns the next line of input, or def once the reader is
// exhausted. Mirrors Read-NervConfigureAnswer/Read-NervAnswer's
// DefaultWhenExhausted behavior.
func (s *session) readLineOr(def string) string {
	if s.in.Scan() {
		return s.in.Text()
	}
	return def
}

// prompt writes label (with a trailing space, no newline) and returns the
// next trimmed line of input, or "" once exhausted. Used for prompts with
// no "keep current" meaning of their own (repo paths, custom model ids).
func (s *session) prompt(label string) string {
	fmt.Fprintf(s.out, "%s ", label)
	return strings.TrimSpace(s.readLineOr(""))
}

// promptExhausted is prompt, but with an explicit non-blank fallback for
// once the reader is exhausted — used only where a genuine mid-file blank
// answer and "the file ran out" must resolve differently (the models
// section's role-selection loop, where blank means "ask again" but
// exhaustion must still terminate the loop). Mirrors the one place
// configure-models.ps1's Read-NervAnswer passes a non-” DefaultWhenExhausted.
func (s *session) promptExhausted(label, defaultWhenExhausted string) string {
	fmt.Fprintf(s.out, "%s ", label)
	return strings.TrimSpace(s.readLineOr(defaultWhenExhausted))
}

// ask prompts label pre-filled with current; a blank answer keeps current.
// Mirrors Get-NervAnswerOrDefault.
func (s *session) ask(label, current string) string {
	fmt.Fprintf(s.out, "%s [%s]: ", label, current)
	ans := strings.TrimSpace(s.readLineOr(""))
	if ans == "" {
		return current
	}
	return ans
}

// choose prompts label with the allowed values and current pre-filled,
// re-prompting on an invalid non-blank answer up to maxInvalidAttempts
// times before giving up and keeping current. Mirrors
// Get-NervChoiceOrDefault, whose own loop never gives up on a real
// terminal; bounded here so a scripted or exhausted reader can never spin.
func (s *session) choose(label string, allowed []string, current string) string {
	for attempt := 0; attempt < maxInvalidAttempts; attempt++ {
		fmt.Fprintf(s.out, "%s (%s) [%s]: ", label, strings.Join(allowed, "|"), current)
		ans := strings.TrimSpace(s.readLineOr(""))
		if ans == "" {
			return current
		}
		if contains(allowed, ans) {
			return ans
		}
		fmt.Fprintf(s.out, "Invalid input. Allowed: %s.\n", strings.Join(allowed, ", "))
	}
	return current
}

// menuChoice prompts a numbered menu (the models section's model/effort/
// phase pickers): a blank answer means "keep current" (chosen=false), and
// an invalid non-blank answer re-prompts up to maxInvalidAttempts times
// before falling back to "keep current" too. Mirrors Read-NervChoice.
func (s *session) menuChoice(label string, allowed []string) (value string, chosen bool) {
	for attempt := 0; attempt < maxInvalidAttempts; attempt++ {
		fmt.Fprintf(s.out, "%s ", label)
		ans := strings.TrimSpace(s.readLineOr(""))
		if ans == "" {
			return "", false
		}
		if contains(allowed, ans) {
			return ans, true
		}
		fmt.Fprintf(s.out, "Invalid input. Allowed: %s, or Enter to keep current.\n", strings.Join(allowed, ", "))
	}
	return "", false
}

// yesRe matches a "yes" confirmation answer, case-insensitively. Mirrors
// every `-match '(?i)^y(es)?$'` check in configure.ps1/configure-models.ps1.
var yesRe = regexp.MustCompile(`(?i)^y(es)?$`)

// yesNo prompts a [Y/n] (defaultYes) or [y/N] confirmation; a blank answer
// resolves to defaultYes.
func (s *session) yesNo(label string, defaultYes bool) bool {
	hint := "y/N"
	if defaultYes {
		hint = "Y/n"
	}
	fmt.Fprintf(s.out, "%s [%s]: ", label, hint)
	ans := strings.TrimSpace(s.readLineOr(""))
	if ans == "" {
		return defaultYes
	}
	return yesRe.MatchString(ans)
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
