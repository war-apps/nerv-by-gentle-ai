package wizard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
	"github.com/war-apps/nerv-gentle-ai/internal/configure"
)

// runReposSection is Section 3: an "Initialize a repository for NERV now?
// (path, or Enter to skip)" loop, asking a repo's base branch, task
// provider, and (for Teamwork) project/tasklist ids, then writing
// .nerv/nerv.yaml through configure.InitRepo — the same write path
// "nerv configure --init-repo" uses. baseBranch/provider are the
// User-config section's own answers (Section 1's $gitBaseBranch/
// $tasksProvider), used as this section's defaults regardless of whether
// Section 1 actually wrote anything. Mirrors configure.ps1's
// "-- Section 3: Repos --" (~1986-2040).
//
// The path-exists/git-toplevel/already-initialized checks below duplicate
// configure.InitRepo's own — necessary here because, unlike the
// non-interactive --init-repo mode, the wizard must decide whether to ask
// the base-branch/provider/project-id questions at all before it has
// anything to hand InitRepo.
func runReposSection(deps Deps, paths configure.Paths, s *session, out io.Writer, baseBranch, provider string) (bool, error) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- Repositories ---")

	allowedProviders, _ := config.AllowedValues("tasks.provider")
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

		repoBaseBranch := s.ask("Base branch for this repo", baseBranch)
		repoProvider := s.choose("Task provider for this repo", allowedProviders, provider)

		req := configure.InitRepoRequest{Path: repoPath, Base: repoBaseBranch, Provider: repoProvider}
		if repoProvider == "teamwork" {
			req.ProjectID = s.ask("Teamwork project id for this repo", "")
			req.TasklistID = s.ask("Teamwork tasklist id for this repo", "")
		}

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
			fmt.Fprintln(out, "Reminder: open Claude Code in that repo and run /nerv:init once to bootstrap gentle-ai's SDD registry if it is missing.")
		}
	}
}
