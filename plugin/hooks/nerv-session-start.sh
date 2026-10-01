#!/usr/bin/env bash
# nerv-session-start.sh
#
# SessionStart hook for the NERV plugin. When the current repo has opted in
# via ".nerv/nerv.yaml" with "enabled: true", prints a short activation
# header that tells the session to load the NERV orchestrator (Ikari)
# protocol itself, via the Skill tool. Every other repo must see no output
# from this hook — NERV stays fully invisible outside opted-in repos.
#
# The header is intentionally short: Claude Code caps a hook's stdout at
# 10,000 characters, and the full protocol (plugin/skills/nerv-orchestrator/
# SKILL.md) is far larger. Printing the whole skill body here would be
# silently truncated, so the hook instead points the session at the skill
# and lets the Skill tool load it in full, on demand, including after any
# context compaction.
#
# Must never fail the session: always exits 0, even on error.

main() {
  local self_dir
  self_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  # shellcheck source=nerv-common.sh
  source "${self_dir}/nerv-common.sh"

  local project_dir
  project_dir="${CLAUDE_PROJECT_DIR:-$PWD}"

  nerv_repo_enabled "$project_dir" || return 0

  local plugin_root="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
  local skill_file="${plugin_root}/skills/nerv-orchestrator/SKILL.md"

  # The skill file must exist for the header's instruction to resolve, even
  # though the header itself never reads or prints its contents.
  if [ ! -r "$skill_file" ]; then
    echo "nerv-session-start: protocol skill not found at ${skill_file}; injecting nothing" >&2
    return 0
  fi

  cat <<'HEADER'
# NERV orchestrator protocol (active: .nerv/nerv.yaml enabled)

NERV is enabled in this repository. This session is the NERV orchestrator (Ikari).

Before your first response in this session, and again after every context compaction, invoke the skill `nerv:nerv-orchestrator` with the Skill tool and follow it as the orchestrator protocol. Do not answer, route, classify, or delegate before it is loaded. If the Skill tool refuses or the skill is missing, say so plainly and continue under the gentle-ai rules; never pretend NERV is active.

The protocol keeps its run-time sections in reference files under the skill's `references/` folder; load each one at the point the protocol names it, not up front.
HEADER

  return 0
}

main || true
exit 0
