package refusal_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/war-apps/nerv-gentle-ai/internal/refusal"
)

func TestError_UnwrapsAndFormats(t *testing.T) {
	inner := errors.New("bogus")
	err := &refusal.Error{Err: inner}

	if err.Error() != "bogus" {
		t.Errorf("Error() = %q, want %q", err.Error(), "bogus")
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is(err, inner) = false, want true (Unwrap must expose Err)")
	}
}

func TestIs_TrueForARefusalAnywhereInTheChain(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &refusal.Error{Err: errors.New("rejected")})
	if !refusal.Is(err) {
		t.Error("Is() = false, want true for a wrapped *refusal.Error")
	}
}

func TestIs_FalseForAnOrdinaryError(t *testing.T) {
	if refusal.Is(errors.New("plain")) {
		t.Error("Is() = true, want false for an ordinary error")
	}
}
