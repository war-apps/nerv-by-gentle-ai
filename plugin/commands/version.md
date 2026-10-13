---
description: Show the next release version of this repository (or of each monorepo project) computed with its own semantic-release configuration
argument-hint: [--branch <name>] [--project <name>] [--offline]
---

# /nerv:version

Show the next semantic version of the current repository, or of each project
of its `release:` section, without changing anything. Read-only.

1. **Run the computation.** Run, passing `$ARGUMENTS` through unchanged:

   ```
   nerv version next --json $ARGUMENTS
   ```

   If `nerv` is not on PATH, tell the user to install it — `curl -fsSL
   https://raw.githubusercontent.com/war-apps/nerv-by-gentle-ai/main/scripts/install.sh
   | bash`, or `go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest`
   — and stop. Do not compute a version by hand or run semantic-release
   yourself.

   By default the command reads origin's release tags and semantic-release
   notes with the user's git credentials (into a throwaway copy, never the
   repository). `--offline` uses the local tags and notes only; mention it
   when the user wants no network access or origin is unreachable.

2. **On failure** (any exit code other than 0), show stderr as it is, in a short
   message. Exit 1 is a refusal (an invalid `release:` section, an unknown
   project — the message lists the declared ones — or not a git repository);
   exit 2 is a usage or environment problem (git, node or npm missing or too
   old, an unpinned plugin, origin unreachable: suggest `--offline`); 130,
   143 or 129 means the run was interrupted. Stop.

3. **Present the result.** stdout is an array (one entry per project, in
   dependency order) when the repository declares several projects and no
   `--project` was given; otherwise one JSON object. Each has `project`,
   `branch` and the outputs of `cycjimmy/semantic-release-action`. Show one
   short table, one row per project, in the user's language:

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
