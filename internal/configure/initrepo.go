package configure

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/atomicfile"
	"github.com/war-apps/nerv-gentle-ai/internal/config"
)

// InitRepoRequest is --init-repo's parameters: the target directory plus
// the optional per-repo overrides configure.ps1's -RepoBase/-RepoProvider/
// -RepoProjectId/-RepoTasklistId carry.
type InitRepoRequest struct {
	Path       string
	Base       string
	Provider   string
	ProjectID  string
	TasklistID string
}

// InitRepo initializes path for NERV by writing <toplevel>/.nerv/nerv.yaml
// (config.FormatProjectFile) — never overwriting an existing project
// config, reporting that case as a warning rather than an error. path must
// exist and be inside a git working tree (resolved with
// "git -C <path> rev-parse --show-toplevel", the same command
// configure.ps1 uses); either failure is a refusal. Mirrors configure.ps1's
// -InitRepo handling (~1347-1393).
func InitRepo(deps Deps, req InitRepoRequest) (Result, error) {
	if _, err := os.Stat(req.Path); err != nil {
		return Result{}, &RefusalError{Err: fmt.Errorf("Path not found: %s", req.Path)}
	}

	if req.Provider != "" {
		allowed, _ := config.AllowedValues("tasks.provider")
		if !contains(allowed, req.Provider) {
			return Result{}, &RefusalError{Err: fmt.Errorf("Invalid -RepoProvider '%s'. Allowed: %s.", req.Provider, strings.Join(allowed, ", "))}
		}
	}

	stdout, _, exitCode, err := deps.Runner.Run(context.Background(), "git", "-C", req.Path, "rev-parse", "--show-toplevel")
	toplevel := strings.TrimSpace(stdout)
	if err != nil || exitCode != 0 || toplevel == "" {
		return Result{}, &RefusalError{Err: fmt.Errorf("Not a git repository: %s", req.Path)}
	}

	repoConfigPath := filepath.Join(toplevel, ".nerv", "nerv.yaml")

	warnings := emptyStrings()
	written := emptyStrings()
	changed := false

	if _, statErr := os.Stat(repoConfigPath); statErr == nil {
		warnings = append(warnings, fmt.Sprintf("%s already initialized; left untouched.", repoConfigPath))
	} else if errors.Is(statErr, fs.ErrNotExist) {
		text := config.FormatProjectFile(config.ProjectValues{
			BaseBranch: req.Base,
			Provider:   req.Provider,
			ProjectID:  req.ProjectID,
			TasklistID: req.TasklistID,
		})
		if _, err := atomicfile.Save(repoConfigPath, []byte(text), atomicfile.Options{}); err != nil {
			return Result{}, err
		}
		written = append(written, repoConfigPath)
		changed = true
	} else {
		return Result{}, statErr
	}

	return Result{
		Changed:    changed,
		Changes:    emptyChanges(),
		Written:    written,
		Warnings:   warnings,
		ConfigPath: repoConfigPath,
		Backup:     nil,
	}, nil
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
