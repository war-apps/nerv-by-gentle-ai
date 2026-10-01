// Package models reads the NERV agent role plugin defaults from the
// embedded agent frontmatter and applies resolved model/effort
// assignments back onto cached agent files on disk.
package models

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
)

// ---------------------------------------------------------------------------
// PluginDefaults
// ---------------------------------------------------------------------------

var (
	frontmatterModelRe  = regexp.MustCompile(`^model:\s*([^#\s]+)`)
	frontmatterEffortRe = regexp.MustCompile(`^effort:\s*([^#\s]+)`)
	lineSplitRe         = regexp.MustCompile(`\r\n|\n`)
)

// PluginDefaults reads the committed model:/effort: frontmatter of every
// agents/<role>.md file in agents (typically nerv.PluginFS()), returning
// role -> config.ModelOverride. A file with no frontmatter, or no
// model:/effort: keys, is simply absent from the result — never an error.
// Mirrors Get-NervPluginDefaults.
func PluginDefaults(agents fs.FS) (map[string]config.ModelOverride, error) {
	entries, err := fs.ReadDir(agents, "agents")
	if err != nil {
		return nil, err
	}

	defaults := map[string]config.ModelOverride{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		role := strings.TrimSuffix(entry.Name(), ".md")

		data, err := fs.ReadFile(agents, "agents/"+entry.Name())
		if err != nil {
			return nil, err
		}

		override, ok := frontmatterDefaults(data)
		if ok {
			defaults[role] = override
		}
	}
	return defaults, nil
}

func frontmatterDefaults(content []byte) (config.ModelOverride, bool) {
	lines := lineSplitRe.Split(string(content), -1)
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return config.ModelOverride{}, false
	}

	var override config.ModelOverride
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			break
		}
		if m := frontmatterModelRe.FindStringSubmatch(lines[i]); m != nil {
			override.Model = strings.TrimSpace(m[1])
		} else if m := frontmatterEffortRe.FindStringSubmatch(lines[i]); m != nil {
			override.Effort = strings.TrimSpace(m[1])
		}
	}

	if override == (config.ModelOverride{}) {
		return config.ModelOverride{}, false
	}
	return override, true
}

// ---------------------------------------------------------------------------
// ApplyToDir
// ---------------------------------------------------------------------------

var (
	frontmatterModelLineRe  = regexp.MustCompile(`^model:\s`)
	frontmatterEffortLineRe = regexp.MustCompile(`^effort:\s`)
	modelValueRe            = regexp.MustCompile(`^model:(\s*)(\S+)(.*)$`)
	effortValueRe           = regexp.MustCompile(`^effort:(\s*)(\S+)(.*)$`)
)

// applyFrontmatter rewrites the model:/effort: lines of one agent file's
// frontmatter (the region between the first two "---" lines) to model and
// effort, preserving any trailing "# comment" on either line verbatim and
// the file's own line-ending style (CRLF or LF). An empty model or effort
// means "leave that key untouched" (matching config.ModelOverride's own
// convention that an empty field is absent). changed is false, and out is
// content unchanged, when the frontmatter is malformed (missing opening
// or closing "---"), there is no model: key at all, or every requested
// value already matches. ok is false when the file was not well-formed
// enough to apply at all (malformed frontmatter or a missing model: key)
// — ApplyToDir counts that as skipped rather than failing the whole run.
func applyFrontmatter(content []byte, model, effort string) (out []byte, changed bool, ok bool) {
	text := string(content)
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	lines := lineSplitRe.Split(text, -1)

	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return content, false, false
	}

	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return content, false, false
	}

	modelLineIdx, effortLineIdx := -1, -1
	for i := 1; i < closeIdx; i++ {
		switch {
		case frontmatterModelLineRe.MatchString(lines[i]):
			modelLineIdx = i
		case frontmatterEffortLineRe.MatchString(lines[i]):
			effortLineIdx = i
		}
	}
	if modelLineIdx < 0 {
		return content, false, false
	}

	modelChanged, effortChanged := false, false

	if model != "" {
		if m := modelValueRe.FindStringSubmatch(lines[modelLineIdx]); m != nil && m[2] != model {
			lines[modelLineIdx] = "model:" + m[1] + model + m[3]
			modelChanged = true
		}
	}

	if effort != "" {
		if effortLineIdx >= 0 {
			if m := effortValueRe.FindStringSubmatch(lines[effortLineIdx]); m != nil && m[2] != effort {
				lines[effortLineIdx] = "effort:" + m[1] + effort + m[3]
				effortChanged = true
			}
		} else {
			inserted := make([]string, 0, len(lines)+1)
			inserted = append(inserted, lines[:modelLineIdx+1]...)
			inserted = append(inserted, "effort: "+effort)
			inserted = append(inserted, lines[modelLineIdx+1:]...)
			lines = inserted
			effortChanged = true
		}
	}

	if !modelChanged && !effortChanged {
		return content, false, true
	}
	return []byte(strings.Join(lines, eol)), true, true
}

// ApplySummary tallies ApplyToDir's per-role outcome, mirroring
// Set-NervAgentFrontmatter's changed/unchanged/skipped counters.
type ApplySummary struct {
	Changed  int
	UpToDate int
	Skipped  int
}

// ApplyToDir applies assignments (role -> config.ModelOverride) to every
// <dir>/<role>.md file, writing back only the files that actually
// changed. A role whose file is missing, or whose frontmatter is
// malformed or lacks a model: key, counts as Skipped rather than failing
// the whole run. Mirrors Set-NervAgentFrontmatter driven over a directory.
func ApplyToDir(dir string, assignments map[string]config.ModelOverride) (ApplySummary, error) {
	var summary ApplySummary

	roles := make([]string, 0, len(assignments))
	for role := range assignments {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	for _, role := range roles {
		assignment := assignments[role]
		path := filepath.Join(dir, role+".md")

		content, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				summary.Skipped++
				continue
			}
			return summary, err
		}

		out, changed, ok := applyFrontmatter(content, assignment.Model, assignment.Effort)
		if !ok {
			summary.Skipped++
			continue
		}
		if !changed {
			summary.UpToDate++
			continue
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return summary, err
		}
		summary.Changed++
	}

	return summary, nil
}
