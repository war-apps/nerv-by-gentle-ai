// Package workflowconfig is the one loader of nerv's two configuration
// files: the user-scope "<os.UserConfigDir()>/ai-workflow/config.yaml" and
// the project-scope "<repo>/.ai-workflow/config.yaml" (see internal/paths
// for the path catalogue). Values merge key by key, with the project
// winning.
//
// The 4.2.0 files, "~/.claude/nerv/nerv.yaml" and "<repo>/.nerv/nerv.yaml",
// are never read as configuration: when only an old file exists, the scope
// is unconfigured and Hints carries one line saying "nerv migrate" moves
// it. The previous files, "<os.UserConfigDir()>/ai-workflow/config.yaml" and
// "<repo>/.ai-workflow/config.yaml", are refused while they exist, so their
// settings are never silently ignored.
package workflowconfig

import (
	"fmt"
	"io"
	"os"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/configstore"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

// Config is the two configuration documents of one home and repository.
type Config struct {
	// UserPath and ProjectPath are the files read; ProjectPath is empty
	// when Load was given no repository.
	UserPath, ProjectPath string
	// UserExists and ProjectExists report whether each file exists.
	UserExists, ProjectExists bool
	// User and Project are the parsed documents (empty when missing).
	User, Project *config.Document
	// Hints are the one-line "nerv migrate" notices for every scope that
	// has only its old file.
	Hints []string
}

// Load reads the user file of home and, when repoRoot is not empty, the
// project file of repoRoot. A previous file (config.yml, .ai-workflow.yml)
// of either scope is a refusal naming "nerv migrate".
func Load(home, repoRoot string) (Config, error) {
	p := paths.Resolve(home)
	if home != "" {
		if err := p.RefusePreviousUser(); err != nil {
			return Config{}, err
		}
	}
	if repoRoot != "" {
		if err := paths.RefusePreviousProject(repoRoot); err != nil {
			return Config{}, err
		}
	}
	cfg := Config{UserPath: p.UserConfig, Project: config.Parse("")}
	var err error
	if cfg.User, cfg.UserExists, err = (configstore.Store{}).Load(p.UserConfig); err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", p.UserConfig, err)
	}
	cfg.Hints = LegacyHints(home, repoRoot)
	if repoRoot == "" {
		return cfg, nil
	}
	cfg.ProjectPath = paths.ProjectConfig(repoRoot)
	if cfg.Project, cfg.ProjectExists, err = (configstore.Store{}).Load(cfg.ProjectPath); err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", cfg.ProjectPath, err)
	}
	return cfg, nil
}

// Scalar resolves a dotted key path, project first, then user.
func (c Config) Scalar(path string) (string, bool) {
	if c.Project != nil {
		if v, found := c.Project.Scalar(path); found {
			return v, true
		}
	}
	if c.User != nil {
		return c.User.Scalar(path)
	}
	return "", false
}

// LegacyHints returns the "nerv migrate" notice of the user scope of home
// and, when repoRoot is not empty, of the project scope of repoRoot, in
// that order, for each scope that has only its old file.
func LegacyHints(home, repoRoot string) []string {
	var hints []string
	if home != "" {
		p := paths.Resolve(home)
		if h := LegacyHint(p.LegacyUserConfig, p.UserConfig); h != "" {
			hints = append(hints, h)
		}
	}
	if repoRoot != "" {
		if h := LegacyHint(paths.LegacyProjectConfig(repoRoot), paths.ProjectConfig(repoRoot)); h != "" {
			hints = append(hints, h)
		}
	}
	return hints
}

// LegacyHint is the one-line notice for an old configuration file that
// exists while its replacement does not; empty otherwise.
func LegacyHint(oldPath, newPath string) string {
	if !isFile(oldPath) || exists(newPath) {
		return ""
	}
	return fmt.Sprintf("nerv: %s is no longer read (configuration now lives in %s); run \"nerv migrate\" to move it", oldPath, newPath)
}

// PrintHints writes each hint of LegacyHints(home, repoRoot) on its own
// line to w.
func PrintHints(w io.Writer, home, repoRoot string) {
	for _, h := range LegacyHints(home, repoRoot) {
		fmt.Fprintln(w, h)
	}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
