package wizard

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// P3.1.2 (EOF aborts): ask, choose, menuChoice, and yesNo must distinguish
// "the reader gave a blank line" (still means keep current / no change,
// unchanged) from "the reader is genuinely exhausted" (aborts with
// ErrInputClosed instead of silently answering as if the line were
// blank). Before this fix, both cases fell through readLineOr's own
// default, which is exactly how a redirected /dev/null (real EOF, no
// lines at all) was silently treated as "keep every default and answer Y
// to every yes/no" (the 2026-09-29 incident).

func TestAsk_BlankLine_KeepsCurrent(t *testing.T) {
	var out bytes.Buffer
	s := newSession(strings.NewReader("\n"), &out)

	got, err := s.ask("Base branch", "develop")
	if err != nil {
		t.Fatalf("ask() error = %v, want nil", err)
	}
	if got != "develop" {
		t.Errorf("ask() = %q, want %q", got, "develop")
	}
}

func TestAsk_EOF_ReturnsErrInputClosed(t *testing.T) {
	var out bytes.Buffer
	s := newSession(strings.NewReader(""), &out)

	got, err := s.ask("Base branch", "develop")
	if !errors.Is(err, ErrInputClosed) {
		t.Fatalf("ask() error = %v, want ErrInputClosed", err)
	}
	if got != "" {
		t.Errorf("ask() value = %q, want empty on EOF", got)
	}
}

func TestChoose_BlankLine_KeepsCurrent(t *testing.T) {
	var out bytes.Buffer
	s := newSession(strings.NewReader("\n"), &out)

	got, err := s.choose("Worktree policy", []string{"ask", "always", "never"}, "ask")
	if err != nil {
		t.Fatalf("choose() error = %v, want nil", err)
	}
	if got != "ask" {
		t.Errorf("choose() = %q, want %q", got, "ask")
	}
}

func TestChoose_EOF_ReturnsErrInputClosed(t *testing.T) {
	var out bytes.Buffer
	s := newSession(strings.NewReader(""), &out)

	_, err := s.choose("Worktree policy", []string{"ask", "always", "never"}, "ask")
	if !errors.Is(err, ErrInputClosed) {
		t.Fatalf("choose() error = %v, want ErrInputClosed", err)
	}
}

func TestChoose_EOFDuringReprompt_ReturnsErrInputClosed(t *testing.T) {
	var out bytes.Buffer
	// One invalid answer, then the reader runs out before a valid one.
	s := newSession(strings.NewReader("bogus\n"), &out)

	_, err := s.choose("Worktree policy", []string{"ask", "always", "never"}, "ask")
	if !errors.Is(err, ErrInputClosed) {
		t.Fatalf("choose() error = %v, want ErrInputClosed", err)
	}
}

func TestMenuChoice_BlankLine_NotChosenNoError(t *testing.T) {
	var out bytes.Buffer
	s := newSession(strings.NewReader("\n"), &out)

	value, chosen, err := s.menuChoice("Model:", []string{"1", "2"})
	if err != nil {
		t.Fatalf("menuChoice() error = %v, want nil", err)
	}
	if chosen {
		t.Error("menuChoice() chosen = true, want false on blank line")
	}
	if value != "" {
		t.Errorf("menuChoice() value = %q, want empty", value)
	}
}

func TestMenuChoice_EOF_ReturnsErrInputClosed(t *testing.T) {
	var out bytes.Buffer
	s := newSession(strings.NewReader(""), &out)

	_, chosen, err := s.menuChoice("Model:", []string{"1", "2"})
	if !errors.Is(err, ErrInputClosed) {
		t.Fatalf("menuChoice() error = %v, want ErrInputClosed", err)
	}
	if chosen {
		t.Error("menuChoice() chosen = true, want false on EOF")
	}
}

func TestYesNo_BlankLine_ReturnsDefault(t *testing.T) {
	var out bytes.Buffer
	s := newSession(strings.NewReader("\n"), &out)

	got, err := s.yesNo("Apply now?", true)
	if err != nil {
		t.Fatalf("yesNo() error = %v, want nil", err)
	}
	if !got {
		t.Error("yesNo() = false, want defaultYes (true) on blank line")
	}
}

func TestYesNo_EOF_ReturnsErrInputClosed(t *testing.T) {
	var out bytes.Buffer
	s := newSession(strings.NewReader(""), &out)

	got, err := s.yesNo("Apply now?", true)
	if !errors.Is(err, ErrInputClosed) {
		t.Fatalf("yesNo() error = %v, want ErrInputClosed", err)
	}
	if got {
		t.Error("yesNo() = true, want false on EOF")
	}
}

// prompt and promptExhausted are unchanged by this fix: they still fall
// back to their own default (never an error) on EOF, matching their
// existing documented behavior for repo paths, custom model ids, and the
// models section's role-selection loop.
func TestPrompt_EOF_StillFallsBackToDefault(t *testing.T) {
	var out bytes.Buffer
	s := newSession(strings.NewReader(""), &out)

	if got := s.prompt("Repo path:"); got != "" {
		t.Errorf("prompt() = %q, want empty on EOF", got)
	}
}
