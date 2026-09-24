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
    local skill_file="${CLAUDE_PLUGIN_ROOT}/skills/nerv-orchestrator/SKILL.md"
    echo "# NERV orchestrator protocol (active: .nerv/nerv.yaml enabled)"
    if [ -f "$skill_file" ]; then
      cat "$skill_file"
    fi
  fi

  return 0
}

main || true
exit 0
