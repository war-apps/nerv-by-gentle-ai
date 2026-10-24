---
description: Show the next release version of this repository (or of each monorepo project) computed by nerv's native version engine from the ship section of .ai-workflow.yml
argument-hint: [--branch <name>] [--project <name>] [--offline]
---

# /ship:version

Show the next semantic version of the current repository, or of each project
of its `ship:` section, without changing anything. Read-only. Load the `ship`
skill first: it is the contract the steps below refer to (§ numbers).

1. **Run the computation.** Run, passing `$ARGUMENTS` through unchanged:

   ```
   nerv version next --json $ARGUMENTS
   ```

   If `nerv` is not on PATH, give the install line of §1 and stop. Never
   compute a version by hand, and never run semantic-release, node or npm:
   the engine is native.

   By default the command reads origin's release tags and channel notes with
   the user's git credentials (into a throwaway copy, never the repository).
   `--offline` uses the local tags and notes only; mention it when the user
   wants no network access or origin is unreachable.

2. **On failure** (any exit code other than 0), show stderr as it is, in a
   short message, with the meaning of the exit code (§3), and stop. For an
   old `release:` section, suggest `/ship:setup` (or `nerv migrate`).

3. **Present the result.** In the user's language, one short table, one row
   per project:

   | Column | From |
   |---|---|
   | Project | `project` (`branch`) |
   | New release | `new_release_published` (yes / no) |
   | Next version | `new_release_version` (blank when no release) |
   | Channel | `new_release_channel` (empty means the default channel) |
   | Tag | `new_release_git_tag` |
   | Last release | `last_release_version` (`last_release_git_tag`), or none |

   "No new release" is a normal result, not an error. Do not print the
   release notes unless the user asks; when asked, show `new_release_notes`
   as Markdown. If the run was not `--offline`, add one line saying origin's
   tags and notes were read; if it was, say the result uses local tags only
   and may differ from CI on prerelease branches.

When the user asks why a version is what it is, launch the `ship:versioner`
agent with the same arguments and the question.
