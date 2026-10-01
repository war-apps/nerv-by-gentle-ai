#!/usr/bin/env bash
# nerv-common.sh
#
# Shared helpers for NERV's Claude Code hooks (nerv-session-start.sh,
# nerv-engram-project.sh). Meant to be sourced, not executed directly.

# nerv_repo_enabled ROOT
#
# Reports, via exit status, whether ROOT/.nerv/nerv.yaml exists and
# carries an "enabled: true" key (allowing surrounding whitespace, CRLF
# line endings, and a trailing comment) on its own line.
nerv_repo_enabled() {
  local root="$1"
  local config_file="${root}/.nerv/nerv.yaml"
  [ -f "$config_file" ] || return 1
  tr -d '\r' <"$config_file" | grep -Eq '^[[:space:]]*enabled:[[:space:]]*true[[:space:]]*(#.*)?$'
}
