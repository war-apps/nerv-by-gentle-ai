// Package skills reads the NERV skills manifest, computes each skill's
// installed status against a Claude Code user-scope skills directory, and
// runs the "npx skills add" installs for missing external skills through
// env.Runner. Mirrors tools/install-skills.ps1.
package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/env"
)

// SchemaID is the manifest schema this package understands.
const SchemaID = "nerv.skills-manifest/v1"

// Skill kinds, matching the manifest's "kind" values.
const (
	KindExternal = "external"
	KindGentleAI = "gentle-ai"
	KindBuiltin  = "builtin"
)

// Status actions, matching Get-NervSkillsStatus's "action" values.
const (
	ActionNone           = "none"
	ActionInstall        = "install"
	ActionVerifyGentleAI = "verify-gentle-ai"
)

// manifestPath is where the manifest lives inside the embedded plugin
// tree (nerv.PluginFS()), i.e. plugin/skills-manifest.json on disk.
const manifestPath = "skills-manifest.json"

// SkillEntry is one manifest entry.
type SkillEntry struct {
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`
	Repo   string   `json:"repo,omitempty"`
	Skill  string   `json:"skill,omitempty"`
	UsedBy []string `json:"used_by"`
}

// Manifest is the parsed nerv.skills-manifest/v1 document.
type Manifest struct {
	Schema string       `json:"schema"`
	Skills []SkillEntry `json:"skills"`
}

// ---------------------------------------------------------------------------
// LoadManifest / LoadManifestFS
// ---------------------------------------------------------------------------

// LoadManifest parses manifest JSON bytes, validating the
// nerv.skills-manifest/v1 schema and each entry's required keys (name,
// kind, used_by; repo additionally for kind "external"). Mirrors
// Read-NervSkillsManifest.
func LoadManifest(data []byte) (*Manifest, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("parse skills manifest: %w", err)
	}

	var schema string
	if raw, ok := top["schema"]; ok {
		_ = json.Unmarshal(raw, &schema)
	}
	if schema != SchemaID {
		return nil, fmt.Errorf("skills manifest has an unexpected or missing 'schema' (expected %s)", SchemaID)
	}

	skillsRaw, ok := top["skills"]
	if !ok {
		return nil, fmt.Errorf("skills manifest is missing a 'skills' array")
	}

	var rawEntries []map[string]json.RawMessage
	if err := json.Unmarshal(skillsRaw, &rawEntries); err != nil {
		return nil, fmt.Errorf("parse skills manifest 'skills' array: %w", err)
	}

	entries := make([]SkillEntry, 0, len(rawEntries))
	for _, raw := range rawEntries {
		for _, key := range []string{"name", "kind", "used_by"} {
			if _, ok := raw[key]; !ok {
				return nil, fmt.Errorf("skills manifest entry is missing required key %q", key)
			}
		}

		var entry SkillEntry
		reencoded, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("parse skills manifest entry: %w", err)
		}
		if err := json.Unmarshal(reencoded, &entry); err != nil {
			return nil, fmt.Errorf("parse skills manifest entry: %w", err)
		}

		switch entry.Kind {
		case KindExternal, KindGentleAI, KindBuiltin:
		default:
			return nil, fmt.Errorf("skills manifest entry %q has unknown kind %q (expected external, gentle-ai, or builtin)", entry.Name, entry.Kind)
		}
		if entry.Kind == KindExternal {
			if _, ok := raw["repo"]; !ok {
				return nil, fmt.Errorf("skills manifest entry %q is kind 'external' but has no 'repo'", entry.Name)
			}
		}

		entries = append(entries, entry)
	}

	return &Manifest{Schema: schema, Skills: entries}, nil
}

// LoadManifestFS reads skills-manifest.json from fsys (typically
// nerv.PluginFS()) and parses it via LoadManifest.
func LoadManifestFS(fsys fs.FS) (*Manifest, error) {
	data, err := fs.ReadFile(fsys, manifestPath)
	if err != nil {
		return nil, err
	}
	return LoadManifest(data)
}

// ---------------------------------------------------------------------------
// IsInstalled / Status
// ---------------------------------------------------------------------------

// IsInstalled reports whether <skillsDir>/<name>/SKILL.md exists. Mirrors
// Test-NervSkillInstalled.
func IsInstalled(skillsDir, name string) bool {
	info, err := os.Stat(filepath.Join(skillsDir, name, "SKILL.md"))
	return err == nil && !info.IsDir()
}

// SkillStatus is one manifest entry's computed status against skillsDir.
type SkillStatus struct {
	Name      string
	Kind      string
	Repo      string
	Skill     string
	Installed bool
	Action    string
}

// Status computes { name, kind, repo, skill, installed, action } for
// every entry in manifest against skillsDir. A builtin entry is always
// reported installed; action is "install" for a missing external skill,
// "verify-gentle-ai" for a missing gentle-ai skill, and "none" otherwise.
// Mirrors Get-NervSkillsStatus.
func Status(manifest *Manifest, skillsDir string) []SkillStatus {
	result := make([]SkillStatus, 0, len(manifest.Skills))
	for _, entry := range manifest.Skills {
		installed := entry.Kind == KindBuiltin || IsInstalled(skillsDir, entry.Name)

		action := ActionNone
		if !installed {
			switch entry.Kind {
			case KindExternal:
				action = ActionInstall
			case KindGentleAI:
				action = ActionVerifyGentleAI
			}
		}

		result = append(result, SkillStatus{
			Name:      entry.Name,
			Kind:      entry.Kind,
			Repo:      entry.Repo,
			Skill:     entry.Skill,
			Installed: installed,
			Action:    action,
		})
	}
	return result
}

// ---------------------------------------------------------------------------
// InstallArgs / InstallPlan
// ---------------------------------------------------------------------------

// InstallArgs builds the argv for "npx <args>" that installs one external
// skill: skills add <repo> [--skill <id>] -g -a claude-code -y. Mirrors
// New-NervSkillInstallArgs.
func InstallArgs(repo, skill string) []string {
	args := []string{"skills", "add", repo}
	if skill != "" {
		args = append(args, "--skill", skill)
	}
	return append(args, "-g", "-a", "claude-code", "-y")
}

// InstallStep is one external skill's npx invocation.
type InstallStep struct {
	Name string
	Args []string
}

// Plan is what to do about every skill whose Status action is not "none":
// run Installs' npx invocations, and surface Remedies to the user for the
// gentle-ai gaps this package never tries to install itself.
type Plan struct {
	Installs []InstallStep
	Remedies []string
}

// InstallPlan turns Status's output into a Plan. Mirrors the per-status
// dispatch in install-skills.ps1's body (install vs. verify-gentle-ai vs.
// none).
func InstallPlan(statuses []SkillStatus) Plan {
	var plan Plan
	for _, s := range statuses {
		switch s.Action {
		case ActionInstall:
			plan.Installs = append(plan.Installs, InstallStep{Name: s.Name, Args: InstallArgs(s.Repo, s.Skill)})
		case ActionVerifyGentleAI:
			plan.Remedies = append(plan.Remedies, fmt.Sprintf(
				"Remedy: run 'gentle-ai install' (or 'gentle-ai sync') to provide gentle-ai skill '%s'.", s.Name))
		}
	}
	return plan
}

// ---------------------------------------------------------------------------
// FilterOnly
// ---------------------------------------------------------------------------

// FilterOnly restricts manifest's entries to the given names (manifest
// order preserved), or returns manifest's full entry list when only is
// empty. An empty error naming every unrecognized name is returned when
// only contains a name absent from manifest. Mirrors the -Only validation
// inlined in install-skills.ps1's body.
func FilterOnly(manifest *Manifest, only []string) ([]SkillEntry, error) {
	if len(only) == 0 {
		return manifest.Skills, nil
	}

	known := make(map[string]bool, len(manifest.Skills))
	for _, e := range manifest.Skills {
		known[e.Name] = true
	}

	var unknown []string
	wanted := make(map[string]bool, len(only))
	for _, name := range only {
		wanted[name] = true
		if !known[name] {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown skill name(s) for -Only: %s", strings.Join(unknown, ", "))
	}

	result := make([]SkillEntry, 0, len(only))
	for _, e := range manifest.Skills {
		if wanted[e.Name] {
			result = append(result, e)
		}
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Install
// ---------------------------------------------------------------------------

// InstallResult tallies Install's outcome.
type InstallResult struct {
	Installed int
	Failed    int
	Failures  []string
}

// Install runs "npx <args>" through runner for every step in plan.Installs,
// counting a launch error or a non-zero exit as a failure. Mirrors the
// -DryRun-less branch of install-skills.ps1's body.
func Install(ctx context.Context, runner env.Runner, plan Plan) InstallResult {
	var result InstallResult
	for _, step := range plan.Installs {
		_, _, exitCode, err := runner.Run(ctx, "npx", step.Args...)
		if err != nil || exitCode != 0 {
			result.Failed++
			result.Failures = append(result.Failures, step.Name)
			continue
		}
		result.Installed++
	}
	return result
}
