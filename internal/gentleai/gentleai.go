// Package gentleai detects the gentle-ai CLI (presence, version, the 3.x
// preflight rule) and reads its claude_phase_assignments state, without
// ever touching a real process or the real home directory outside a
// caller-supplied env.Runner / path.
package gentleai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
	"github.com/war-apps/nerv-gentle-ai/internal/env"
)

// Preflight is the result of checking gentle-ai's presence and version.
// Found means "gentle-ai --version" produced any non-blank output at all
// — even when that output has no parseable MAJOR.MINOR.PATCH token, in
// which case Version stays empty. OK means the parsed major version is 3
// (NERV requires gentle-ai 3.x; tested against 3.7.0).
type Preflight struct {
	Found   bool
	Version string
	OK      bool
}

// ToolStatus is the presence-only status of a tool located via PATH
// (engram, claude — the -Print prerequisites shape never reports a
// version for these).
type ToolStatus struct {
	Found bool
}

// PrerequisitesStatus is -Print's `prerequisites` shape.
type PrerequisitesStatus struct {
	GentleAI Preflight
	Engram   ToolStatus
	Claude   ToolStatus
}

var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

// CheckPreflight runs "gentle-ai --version" through runner and computes
// the Preflight result: Found, its parsed Version (when parseable), and
// whether it satisfies NERV's 3.x requirement.
func CheckPreflight(ctx context.Context, runner env.Runner) Preflight {
	line, found := firstOutputLine(ctx, runner)
	if !found {
		return Preflight{}
	}

	match := versionRe.FindString(line)
	if match == "" {
		return Preflight{Found: true}
	}

	major := majorOf(match)
	return Preflight{Found: true, Version: match, OK: major == 3}
}

func majorOf(semver string) int {
	major, _ := strconv.Atoi(strings.SplitN(semver, ".", 2)[0])
	return major
}

// firstOutputLine runs "gentle-ai --version" and returns the first
// non-blank line of its combined stdout/stderr output (mirroring the
// PowerShell preflight's `2>&1 | Select-Object -First 1`), and whether any
// such line exists at all. A launch failure (runner err != nil) always
// reports found=false, matching the script's try/catch around a missing
// executable.
func firstOutputLine(ctx context.Context, runner env.Runner) (line string, found bool) {
	stdout, stderr, _, err := runner.Run(ctx, "gentle-ai", "--version")
	if err != nil {
		return "", false
	}

	line = firstLine(stdout)
	if strings.TrimSpace(line) == "" {
		line = firstLine(stderr)
	}
	return line, strings.TrimSpace(line) != ""
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// PhaseAssignments reads path (typically ~/.gentle-ai/state.json) and
// returns its claude_phase_assignments map as phase -> config.PhaseAssignment.
// A missing file returns an empty map with no error; malformed JSON
// returns an error. Mirrors the state-file half of
// Resolve-NervModelAssignments.
func PhaseAssignments(path string) (map[string]config.PhaseAssignment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]config.PhaseAssignment{}, nil
		}
		return nil, err
	}

	var state struct {
		ClaudePhaseAssignments map[string]struct {
			Model  string `json:"model"`
			Effort string `json:"effort"`
		} `json:"claude_phase_assignments"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	result := make(map[string]config.PhaseAssignment, len(state.ClaudePhaseAssignments))
	for phase, entry := range state.ClaudePhaseAssignments {
		result[phase] = config.PhaseAssignment{Model: entry.Model, Effort: entry.Effort}
	}
	return result, nil
}

// Prerequisites computes -Print's `prerequisites` shape: gentle-ai's
// version preflight via runner, plus engram/claude's plain PATH presence
// via lookPath. Mirrors Get-NervPrerequisitesStatus.
func Prerequisites(ctx context.Context, runner env.Runner, lookPath func(name string) (string, error)) PrerequisitesStatus {
	_, engramErr := lookPath("engram")
	_, claudeErr := lookPath("claude")

	return PrerequisitesStatus{
		GentleAI: CheckPreflight(ctx, runner),
		Engram:   ToolStatus{Found: engramErr == nil},
		Claude:   ToolStatus{Found: claudeErr == nil},
	}
}
