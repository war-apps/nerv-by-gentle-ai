package wizard

import (
	"context"
	"fmt"
	"io"

	"github.com/war-apps/nerv-gentle-ai/internal/configure"
	"github.com/war-apps/nerv-gentle-ai/internal/skills"
)

// offerSkillsInstall is Section 0b: offers to install the skills the
// plugin references (missing ones only), via internal/skills — the same
// package "nerv skills" itself drives. A declined offer, a missing/
// unreadable manifest, or an install failure is reported but never fails
// the wizard. Mirrors configure.ps1's "-- Section 0b: Required skills --"
// (~1527-1546).
//
// Deviation from the port brief: the script's remedy text names
// `pwsh tools/install-skills.ps1 -DryRun`; that script no longer exists
// (P2), so the declined-offer message instead points at `nerv skills
// --dry-run`.
func offerSkillsInstall(ctx context.Context, deps Deps, paths configure.Paths, s *session, out io.Writer) error {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- Required skills ---")

	install, err := s.yesNo("Install the skills the plugin references (missing ones only, via npx skills add -g)?", true)
	if err != nil {
		return err
	}
	if !install {
		fmt.Fprintln(out, "Skipped. Run `nerv skills` later (add --dry-run to preview).")
		return nil
	}

	manifest, err := skills.LoadManifestFS(deps.FS)
	if err != nil {
		fmt.Fprintf(out, "Warning: could not read the skills manifest: %v\n", err)
		return nil
	}

	statuses := skills.Status(manifest, paths.SkillsDir)
	plan := skills.InstallPlan(statuses)
	for _, remedy := range plan.Remedies {
		fmt.Fprintln(out, remedy)
	}

	result := skills.Install(ctx, deps.Runner, plan)
	fmt.Fprintf(out, "skills: %d installed, %d failed\n", result.Installed, result.Failed)
	if result.Failed > 0 {
		fmt.Fprintln(out, "Warning: some skills failed to install; see the lines above.")
	}
	return nil
}
