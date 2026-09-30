package wizard

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

// maxInvalidAttempts bounds every re-prompt loop below at 3 attempts, so a
// scripted or exhausted reader can never spin forever on invalid input:
// once exhausted, the prompt falls back to its "keep current"/"no change"
// answer instead.
const maxInvalidAttempts = 3

// ErrInputClosed is returned by ask, choose, menuChoice, and yesNo when
// the underlying reader reaches genuine EOF before answering — as
// opposed to a blank line, which every one of those helpers still treats
// as "keep current"/"no change", unchanged. Run stops immediately on this
// error: it writes nothing and runs no command past the point where it
// occurred.
//
// Before this sentinel existed, EOF and a blank line were
// indistinguishable to every helper (both fell through readLineOr's own
// default), which is exactly how a redirected /dev/null (real EOF, no
// lines at all) got silently treated as "keep every default and answer Y
// to every yes/no" — the 2026-09-29 incident that ran `claude plugin
// uninstall/install` and `npx skills add -g` for real. --answers files
// that end early must abort instead of accepting defaults for whatever
// prompts they never actually answered.
var ErrInputClosed = errors.New("input ended before the wizard finished; nothing was written")

// session drives every prompt in one wizard run from a single shared
// line-based reader, so answers (whether typed live or supplied by an
// --answers file) are consumed in order exactly once.
type session struct {
	in  *bufio.Scanner
	out io.Writer
}

func newSession(in io.Reader, out io.Writer) *session {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	return &session{in: scanner, out: out}
}

// readLine returns the next line of input and eof=false, or eof=true once
// the reader is genuinely exhausted (as opposed to a blank line, which
// Scan still reports as a successful read of the empty string).
func (s *session) readLine() (line string, eof bool) {
	if s.in.Scan() {
		return s.in.Text(), false
	}
	return "", true
}

// readLineOr returns the next line of input, or def once the reader is
// exhausted. Mirrors Read-NervConfigureAnswer/Read-NervAnswer's
// DefaultWhenExhausted behavior. Used only by prompt/promptExhausted,
// whose own EOF-as-default behavior is unchanged by ErrInputClosed.
func (s *session) readLineOr(def string) string {
	line, eof := s.readLine()
	if eof {
		return def
	}
	return line
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
// exhaustion must still terminate the loop).
func (s *session) promptExhausted(label, defaultWhenExhausted string) string {
	fmt.Fprintf(s.out, "%s ", label)
	return strings.TrimSpace(s.readLineOr(defaultWhenExhausted))
}

// promptRequired writes label (with a trailing space, no newline) and
// returns the next trimmed line of input. Unlike prompt, which silently
// treats a genuinely exhausted reader as "", it returns ErrInputClosed —
// for a re-prompt loop that must terminate on real EOF instead of looping
// forever on an empty answer (the models section's custom model id
// sub-loop). A blank line (not exhaustion) still returns "", nil: the
// caller decides what blank means.
func (s *session) promptRequired(label string) (string, error) {
	fmt.Fprintf(s.out, "%s ", label)
	line, eof := s.readLine()
	if eof {
		return "", ErrInputClosed
	}
	return strings.TrimSpace(line), nil
}

// ask prompts label pre-filled with current; a blank answer keeps current.
// Mirrors Get-NervAnswerOrDefault. Returns ErrInputClosed if the reader is
// genuinely exhausted rather than silently keeping current.
func (s *session) ask(label, current string) (string, error) {
	fmt.Fprintf(s.out, "%s [%s]: ", label, current)
	line, eof := s.readLine()
	if eof {
		return "", ErrInputClosed
	}
	ans := strings.TrimSpace(line)
	if ans == "" {
		return current, nil
	}
	return ans, nil
}

// choose prompts label with the allowed values and current pre-filled,
// re-prompting on an invalid non-blank answer up to maxInvalidAttempts
// times before giving up and keeping current. Mirrors
// Get-NervChoiceOrDefault, whose own loop never gives up on a real
// terminal; bounded here so a scripted or exhausted reader can never spin.
// Returns ErrInputClosed if the reader is genuinely exhausted (at the
// first prompt or during a re-prompt) rather than silently keeping
// current.
func (s *session) choose(label string, allowed []string, current string) (string, error) {
	for attempt := 0; attempt < maxInvalidAttempts; attempt++ {
		fmt.Fprintf(s.out, "%s (%s) [%s]: ", label, strings.Join(allowed, "|"), current)
		line, eof := s.readLine()
		if eof {
			return "", ErrInputClosed
		}
		ans := strings.TrimSpace(line)
		if ans == "" {
			return current, nil
		}
		if slices.Contains(allowed, ans) {
			return ans, nil
		}
		fmt.Fprintf(s.out, "Invalid input. Allowed: %s.\n", strings.Join(allowed, ", "))
	}
	return current, nil
}

// menuChoice prompts a numbered menu (the models section's model/effort/
// phase pickers): a blank answer means "keep current" (chosen=false), and
// an invalid non-blank answer re-prompts up to maxInvalidAttempts times
// before falling back to "keep current" too. Mirrors Read-NervChoice.
// Returns ErrInputClosed if the reader is genuinely exhausted rather than
// silently keeping current.
func (s *session) menuChoice(label string, allowed []string) (value string, chosen bool, err error) {
	for attempt := 0; attempt < maxInvalidAttempts; attempt++ {
		fmt.Fprintf(s.out, "%s ", label)
		line, eof := s.readLine()
		if eof {
			return "", false, ErrInputClosed
		}
		ans := strings.TrimSpace(line)
		if ans == "" {
			return "", false, nil
		}
		if slices.Contains(allowed, ans) {
			return ans, true, nil
		}
		fmt.Fprintf(s.out, "Invalid input. Allowed: %s, or Enter to keep current.\n", strings.Join(allowed, ", "))
	}
	return "", false, nil
}

// yesRe matches a "yes" confirmation answer, case-insensitively.
var yesRe = regexp.MustCompile(`(?i)^y(es)?$`)

// yesNo prompts a [Y/n] (defaultYes) or [y/N] confirmation; a blank
// answer resolves to defaultYes. Returns ErrInputClosed if the reader is
// genuinely exhausted rather than silently resolving to defaultYes — the
// fix for the incident where an EOF stdin (redirected /dev/null) made
// every confirmation answer "yes".
func (s *session) yesNo(label string, defaultYes bool) (bool, error) {
	hint := "y/N"
	if defaultYes {
		hint = "Y/n"
	}
	fmt.Fprintf(s.out, "%s [%s]: ", label, hint)
	line, eof := s.readLine()
	if eof {
		return false, ErrInputClosed
	}
	ans := strings.TrimSpace(line)
	if ans == "" {
		return defaultYes, nil
	}
	return yesRe.MatchString(ans), nil
}
