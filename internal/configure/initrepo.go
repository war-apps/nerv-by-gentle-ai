package configure

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/atomicfile"
	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
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

// InitRepo initializes path for NERV by writing <toplevel>/.ai-workflow.yml
// (config.FormatProjectFile, plus each ProviderConfig entry under
// tasks.providers.<provider>.<key>) and, when missing, the runtime folder's
// <toplevel>/.nerv/.gitignore of "*" — never overwriting or editing an
// existing project config, reporting that case as a warning rather than an
// error. A pre-rename <toplevel>/.nerv/nerv.yaml is not a project config:
// it is left alone and reported in a warning that "nerv migrate" moves it.
// path must exist and be inside a git working tree (resolved with "git -C
// <path> rev-parse --show-toplevel"); either failure, like an invalid
// ProviderConfig entry, is a refusal.
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

	repoConfigPath := paths.ProjectConfig(toplevel)

	warnings := emptyStrings()
	if hint := workflowconfig.LegacyHint(paths.LegacyProjectConfig(toplevel), repoConfigPath); hint != "" {
		warnings = append(warnings, hint)
	}
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
		ignore, err := paths.EnsureRuntimeIgnored(toplevel)
		if err != nil {
			return Result{}, err
		}
		if ignore != "" {
			written = append(written, ignore)
		}
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
