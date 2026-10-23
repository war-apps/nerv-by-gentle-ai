package configure

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/atomicfile"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// InitRepoRequest is --init-repo's parameters: the target directory plus
// the optional per-repo overrides (base branch, and task provider settings
// as "<provider>.<key>=<value>" entries checked against the embedded
// provider manifests in Deps.ExtensionsFS).
type InitRepoRequest struct {
	Path           string
	Base           string
	ProviderConfig []string
}

// InitRepo initializes path for NERV by writing <toplevel>/.nerv/nerv.yaml
// (config.FormatProjectFile, plus each ProviderConfig entry under
// tasks.providers.<provider>.<key>) — never overwriting or editing an
// existing project config, reporting that case as a warning rather than an
// error. path must exist and be inside a git working tree (resolved with
// "git -C <path> rev-parse --show-toplevel"); either failure, like an
// invalid ProviderConfig entry, is a refusal.
func InitRepo(deps Deps, req InitRepoRequest) (Result, error) {
	if _, err := os.Stat(req.Path); err != nil {
		return Result{}, &RefusalError{Err: fmt.Errorf("Path not found: %s", req.Path)}
	}

	settings, err := ParseProviderConfig(deps.ExtensionsFS, req.ProviderConfig)
	if err != nil {
		return Result{}, err
	}

	stdout, _, exitCode, err := deps.Runner.Run(context.Background(), "git", "-C", req.Path, "rev-parse", "--show-toplevel")
	toplevel := strings.TrimSpace(stdout)
	if err != nil || exitCode != 0 || toplevel == "" {
		return Result{}, &RefusalError{Err: fmt.Errorf("Not a git repository: %s", req.Path)}
	}

	repoConfigPath := filepath.Join(toplevel, ".nerv", "nerv.yaml")

	warnings := emptyStrings()
	written := emptyStrings()
	changes := emptyChanges()
	changed := false

	if _, statErr := os.Stat(repoConfigPath); statErr == nil {
		warnings = append(warnings, fmt.Sprintf("%s already initialized; left untouched.", repoConfigPath))
	} else if errors.Is(statErr, fs.ErrNotExist) {
		doc := config.Parse(config.FormatProjectFile(config.ProjectValues{BaseBranch: req.Base}))
		changes = applyProviderSettings(doc, settings)
		if _, err := atomicfile.Save(repoConfigPath, []byte(doc.String()), atomicfile.Options{}); err != nil {
			return Result{}, err
		}
		written = append(written, repoConfigPath)
		changed = true
	} else {
		return Result{}, statErr
	}

	return Result{
		Changed:    changed,
		Changes:    changes,
		Written:    written,
		Warnings:   warnings,
		ConfigPath: repoConfigPath,
		Backup:     nil,
	}, nil
}
