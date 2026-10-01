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
// the wizard. A declined offer points the user at `nerv skills --dry-run`
// to preview the same install later.
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

	report, err := skills.Run(ctx, deps.Runner, deps.FS, paths.SkillsDir, skills.Options{})
	if err != nil {
		fmt.Fprintf(out, "Warning: could not read the skills manifest: %v\n", err)
		return nil
	}

	for _, remedy := range report.Plan.Remedies {
		fmt.Fprintln(out, remedy)
	}

	fmt.Fprintf(out, "skills: %d installed, %d failed\n", report.Result.Installed, report.Result.Failed)
	if report.Result.Failed > 0 {
		fmt.Fprintln(out, "Warning: some skills failed to install; see the lines above.")
	}
	return nil
}
