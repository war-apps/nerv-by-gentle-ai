---
description: Release and deploy a release artifact with nerv deploy — the latest local artifact or the one you name; always a dry run first, the release commit, tag, push, targets and git host release only after the user's explicit yes
argument-hint: [<artifact dir> | <project>[@<version>]] [--target <name> ...]
---

# /deploy:run

Turn a local artifact that `/release:build` left into a release: the release
commit, the tag and the channel note pushed to origin, every target's
publish, deploy and verify, and the git host release. Load the `deploy`
skill first: it is the contract the steps below refer to (§ numbers).

**Arguments.** `$ARGUMENTS` may be empty, or hold one artifact selector and
any number of `--target <name>` (a non-empty value not starting with `-`).
The selector is an artifact directory (`.nerv/artifacts/api/1.4.0`), a
project (`api`) or a project and version (`api@1.4.0`). Anything else
(including `--yes`, `--dry-run`, `--rebase`, `--json`, `--cwd`): say which
part is not accepted and stop without running `nerv`.

1. **Pick the artifact.** List `.nerv/artifacts/<project>/<version>/`
   directories that hold a `manifest.json`.
   - None: say there is no artifact and that `/release:build` makes one; stop.
   - A directory or `project@version` names one: use it; if it does not
     exist, list the ones there are and stop.
   - Empty, or a project only: take the newest by the manifest's
     `created_at` (of that project, or of all). When several projects have
     artifacts and none was named, show them (project, version, channel,
     created) and ask which one; stop and wait.

   Read its `manifest.json` and say which artifact was picked: project,
   version, tag, channel, branch, base commit.

2. **Rehearse first, always.** Run:

   ```
   nerv deploy --artifact <dir> --dry-run --json [--target <name> ...]
   ```

   If `nerv` is not on PATH, give the install line of §1 and stop.

3. **On a refusal or failure, stop.** Show `error` and the steps as they
   are, with the meaning of the exit code (§5). No `deploy:` section, or a
   host or target without its provider: suggest `/deploy:setup`. A
   preflight refusal names the missing tool or value; say what to install
   or set. Origin moved: say that `--rebase` re-applies the release on the
   new head but tags code that was never built from it, and that releasing
   again is the safe path; run it only if the user explicitly asks for the
   rebase (then rehearse again with `--rebase`).

4. **Present the plan.** One table, one row per step (`name`, `status`,
   `detail`), then one line: what the real run will push (branch, tag,
   note) to `deploy.git.remote`, which targets it deploys in order, and
   whether a git host release is created.

5. **Ask, and wait.** Ask the user explicitly whether to deploy exactly
   this, naming project, version, tag and targets. Only an explicit yes
   counts; anything else means do not run it.

6. **Deploy, only after that yes.** Run the same command without
   `--dry-run`, with `--yes` (and `--rebase` only if the user asked for it).
   The `deploy:deployer` agent can run steps 2 and 6 in the background
   (`MODE: rehearse`, then `MODE: deploy` with `APPROVED: <project>
   <version> <tag>`); the question in step 5 always stays in this session.

7. **Report.** In the user's language: one row per step with its status.
   Name the **pending** steps (`not-run`) and, for a failed one, its
   `detail` and what to fix. Say that running the same `/deploy:run` again
   is safe and resumes after the steps that were done.
