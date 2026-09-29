// Package engram checks or creates the shared "nerv" Engram knowledge-base
// project through the "engram" CLI, without ever failing the caller: every
// problem surfaces as a warning message in the returned Result. Mirrors
// the Engram step of tools/install.ps1 (~795-846).
package engram

import (
	"context"
	"fmt"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/env"
)

const (
	projectName = "nerv"
	description = "Shared Engram project for NERV runs: precedents (Misato rulings, Fuyutsuki vetoes, MAGI vote results, Kaji audit findings) mirrored from every NERV-governed repository."
)

// Result reports what EnsureKnowledgeBase did: human-readable progress
// Messages, and Warnings for anything that went wrong (engram missing on
// PATH, a failed "projects list" or "save" call). Neither ever aborts the
// caller's own workflow.
type Result struct {
	Messages []string
	Warnings []string
}

// EnsureKnowledgeBase verifies (or creates) the "nerv" Engram
// knowledge-base project. lookPath resolves "engram" on PATH — when it is
// not found, the whole check is skipped with a single warning. Otherwise
// "engram projects list" is run through runner; when its first-column
// output already lists "nerv", nothing else happens. Otherwise "engram
// save ... --project nerv --type manual" creates it. Every failure (a
// launch error, a non-zero exit) is reported as a Warning, never as a Go
// error.
func EnsureKnowledgeBase(ctx context.Context, runner env.Runner, lookPath func(name string) (string, error)) Result {
	var result Result

	if _, err := lookPath("engram"); err != nil {
		result.Warnings = append(result.Warnings, "engram not found on PATH; could not verify the 'nerv' Engram knowledge base.")
		return result
	}

	stdout, stderr, exitCode, err := runner.Run(ctx, "engram", "projects", "list")
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("engram projects list could not run: %v", err))
		return result
	}
	if exitCode != 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("engram projects list exited with code %d: %s", exitCode, firstNonEmpty(stdout, stderr)))
		return result
	}

	if projectListed(stdout, projectName) {
		result.Messages = append(result.Messages, "Engram 'nerv' knowledge base already exists.")
		return result
	}

	_, saveStderr, saveExitCode, saveErr := runner.Run(ctx, "engram", "save",
		"NERV knowledge base", description,
		"--project", projectName, "--type", "manual")
	if saveErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("engram save could not run: %v", saveErr))
		return result
	}
	if saveExitCode != 0 {
		_ = saveStderr
		result.Warnings = append(result.Warnings, "engram save exited with a non-zero code; could not create the 'nerv' Engram knowledge base.")
		return result
	}

	result.Messages = append(result.Messages, "Engram 'nerv' knowledge base created.")
	return result
}

// projectListed reports whether any line of stdout's first whitespace
// token equals name, mirroring install.ps1's per-line first-token scan of
// "engram projects list" output.
func projectListed(stdout, name string) bool {
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) > 0 && fields[0] == name {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
