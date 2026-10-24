package wizard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/configure"
)

// runReposSection is Section 3: an "Initialize a repository for NERV now?
// (path, or Enter to skip)" loop, asking a repo's base branch and the
// repo-scope settings of the enabled task providers (from their manifests),
// then writing .nerv/nerv.yaml through configure.InitRepo — the same write
// path "nerv configure --init-repo --repo-provider-config" uses. baseBranch
// is the User-config section's own answer, used as this section's default
// regardless of whether Section 1 actually wrote anything.
//
// The path-exists/git-toplevel/already-initialized checks below duplicate
// configure.InitRepo's own — necessary here because, unlike the
// non-interactive --init-repo mode, the wizard must decide whether to ask
// the base-branch and provider questions at all before it has
// anything to hand InitRepo.
func runReposSection(deps Deps, s *session, out io.Writer, baseBranch string, providers []configure.Provider) (bool, error) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- Repositories ---")

	anyChanged := false

	for {
		repoPath := s.prompt("Initialize a repository for NERV now? (path, or Enter to skip)")
		if repoPath == "" {
			return anyChanged, nil
		}

		if _, err := os.Stat(repoPath); err != nil {
			fmt.Fprintf(out, "Path not found: %s\n", repoPath)
			continue
		}

		stdout, _, exitCode, runErr := deps.Runner.Run(context.Background(), "git", "-C", repoPath, "rev-parse", "--show-toplevel")
		toplevel := strings.TrimSpace(stdout)
		if runErr != nil || exitCode != 0 || toplevel == "" {
			fmt.Fprintf(out, "Not a git repository: %s\n", repoPath)
			continue
		}

		repoConfigPath := filepath.Join(toplevel, ".nerv", "nerv.yaml")
		if _, err := os.Stat(repoConfigPath); err == nil {
			fmt.Fprintf(out, "%s already initialized, left untouched.\n", repoConfigPath)
			continue
		}

		repoBaseBranch, err := s.ask("Base branch for this repo", baseBranch)
		if err != nil {
			return anyChanged, err
		}
		providerConfig, err := askRepoProviderKeys(s, providers)
		if err != nil {
			return anyChanged, err
		}

		req := configure.InitRepoRequest{Path: repoPath, Base: repoBaseBranch, ProviderConfig: providerConfig}

		result, err := configure.InitRepo(deps, req)
		if err != nil {
			var refusal *configure.RefusalError
			if errors.As(err, &refusal) {
				fmt.Fprintln(out, err.Error())
				continue
			}
			return anyChanged, err
		}

		for _, w := range result.Warnings {
			fmt.Fprintln(out, w)
		}
		for _, w := range result.Written {
			fmt.Fprintf(out, "Written: %s\n", w)
			anyChanged = true
		}
		if result.Changed {
			fmt.Fprintln(out, "Reminder: open Claude Code in that repo and run /nerv:init once to create openspec/config.yaml and the skill registry if they are missing.")
		}
	}
}
