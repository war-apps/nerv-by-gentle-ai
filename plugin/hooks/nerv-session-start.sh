#!/usr/bin/env bash
# nerv-session-start.sh
#
# SessionStart hook for the NERV plugin. Injects the NERV orchestrator
# (Ikari) protocol into the session ONLY when the current repo has opted in
# via ".nerv/nerv.yaml" with "enabled: true". Every other repo must see no
# output from this hook — NERV stays fully invisible outside opted-in repos.
#
# Must never fail the session: always exits 0, even on error.

main() {
  local project_dir
  project_dir="${CLAUDE_PROJECT_DIR:-$PWD}"

  local config_file="${project_dir}/.nerv/nerv.yaml"

  [ -f "$config_file" ] || return 0

  # Strip CRLF line endings before matching, then check for an "enabled: true"
  # key (allowing surrounding whitespace) on its own line.
  if tr -d '\r' < "$config_file" | grep -Eq '^[[:space:]]*enabled:[[:space:]]*true[[:space:]]*(#.*)?$'; then
    local plugin_root="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
    local skill_file="${plugin_root}/skills/nerv-orchestrator/SKILL.md"

    if [ ! -r "$skill_file" ]; then
      echo "nerv-session-start: protocol skill not found at ${skill_file}; injecting nothing" >&2
      return 0
    fi

    echo "# NERV orchestrator protocol (active: .nerv/nerv.yaml enabled)"
    cat "$skill_file"
  fi

  return 0
}

main || true
exit 0
