---
description: Run the Docker pipeline (validate, test, release, docker, sync, deploy) with nerv pipeline run — always a dry run first; release, push, merge-back and deploy only after the user's explicit yes
argument-hint: [--from <step> | --only <step>] (nothing else is accepted)
---

# /nerv:pipeline

Run the repository's Docker pipeline from this machine, as
dim.comprobantes.api's CI/CD does: `validate` (format check) → `test`
(build, tests, coverage) → `release` (semantic-release on origin) →
`docker` (build and push the images) → `sync` (merge the release back into
develop and `release/*`) → `deploy` (update the containers over SSH). Each
step is its own `nerv pipeline <step>` command with its own records and
checks; `nerv pipeline run` runs them in that order and stops at the first
one that does not pass. The configuration is the `pipeline:` section of the
local `.nerv/nerv.yaml`; its reference is `docs/pipeline.md`.

**Accepted arguments.** Check `$ARGUMENTS` before running anything: it may
be empty, or hold exactly one of `--from <step>` and `--only <step>`, where
`<step>` is one of `validate`, `test`, `release`, `docker`, `sync`,
`deploy`. Anything else (including `--yes`, `--dry-run`, `--expect`,
`--cwd`, `--json`, extra words): tell the user which part is not accepted,
that `/nerv:pipeline` takes only `--from` or `--only`, and stop without
running `nerv`. Below, `<args>` means exactly the accepted argument. Never
add flags other than the ones written here.

Talk to the user in their language.

1. **Preconditions.** Run `nerv version`. If `nerv` is not on PATH, tell
   the user to install it — `curl -fsSL
   https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh
   | bash`, or `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest`
   — and stop. Never run semantic-release, `docker push`, `git push`, `ssh`
   or the hosts' compose scripts yourself: only `nerv pipeline`.

2. **Rehearse first, always.** Run:

   ```
   nerv pipeline run --dry-run --json <args>
   ```

   It is real work: validate and test run in the working tree, the release
   is computed on origin's state, the images are built (not pushed), the
   merge-backs are previewed and the deploy data is read (no SSH
   connection). It can take minutes. stdout is one JSON document `{status,
   dry_run, steps: [{step, status, exit, error}], error}`; every stderr
   line is prefixed `[pipeline:<step>]`.

3. **On a step that did not pass, stop.** `status` other than `passed`
   names the failing step in `error`; its `exit` is 1 for a refusal (an
   invalid or missing `pipeline:` section, a record of an earlier step
   missing or not for this release, the local branch not origin's), 2 for
   a failure, 128 + a signal when interrupted; the later steps are
   `not-run`. Show `error` and the relevant stderr lines as they are, in a
   short message, and stop. Never work around a refusal by running a later
   step on its own.

4. **Present the plan.** Read the step records the dry run wrote under
   `.nerv/artifacts/pipeline/` and show, in one short summary:
   - `release.json`: `published` (false: there is no new release, and the
     docker, sync and deploy steps have nothing to do), `version`,
     `channel`, `git_tag`, `branch`;
   - `docker.json`: each image's `name` and `tags`;
   - `sync.json`: each target `branch`, its `source` and its status
     (`would-merge`, `would-conflict` with `conflicts`, `up-to-date`);
   - `deploy.json`: each environment, its `status` (`would-deploy` or
     `skipped` with `reason`), `container` and `host`, and any blue-green
     warning from stderr.

   Say what the real run will do that cannot be undone: release (release
   commit, tag, push to origin, GitHub release), push the images, push the
   merge-back commits, and update the listed containers.

5. **Ask, and wait.** Ask the user explicitly whether to run exactly this,
   naming the version and tag, the images, the branches to merge into and
   the environments to deploy, then stop and wait for the answer. The
   CLI's own `[y/N]` prompts cannot be answered from this shell, which is
   why the real run needs `--yes`. Only an explicit yes (e.g. "yes", "run
   it") counts; anything else, silence or a question means do not run:
   answer the question or confirm that nothing was released, pushed or
   deployed. If the user approves everything except the deploy, run the
   steps up to sync one at a time instead (step 6 with `--only release`,
   then `--only docker`, then `--only sync`), never `deploy`.

6. **Run, only after that yes, bound to what was approved.** Run:

   ```
   nerv pipeline run --yes --json <args> --expect <project>=<version>
   ```

   with the `project` and `version` of the dry-run `release.json` (omit
   `--expect` when `published` was false, or when the run does not
   include the release step). The release step reads origin again; if the
   release it computes is not the approved one it refuses (exit 1) before
   releasing anything, and nothing after it runs. Tell the user this
   before running it.

7. **Report the result document** in the user's language: `status` and
   `error`, then one row per step with `status` and `exit`, and the
   details of the records the run wrote (`release.json`, `docker.json`,
   `sync.json`, `deploy.json`, `run.json`). For a failed step quote its
   `error` and any `action` in its record without rewording the
   instructions. Never run `nerv pipeline run` again on your own after a
   failure: a step whose record shows it already pushed or released must
   not be repeated blindly; say what the record shows and what the user
   can run next (for example `/nerv:pipeline --from <step>`).
