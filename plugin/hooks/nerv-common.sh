#!/usr/bin/env bash
# nerv-common.sh
#
# Shared helpers for NERV's Claude Code hooks (nerv-session-start.sh,
# nerv-engram-project.sh). Meant to be sourced, not executed directly.

# nerv_file_enabled FILE
#
# Reports, via exit status, whether FILE exists and carries a top-level
# "enabled: true" key on its own line, at column 0 (allowing whitespace
# after the colon and the value, CRLF line endings, and a trailing comment).
# An indented "enabled: true" belongs to a nested section (a plugin's own
# flag) and does not count.
nerv_file_enabled() {
  local config_file="$1"
  [ -f "$config_file" ] || return 1
  tr -d '\r' <"$config_file" | grep -Eq '^enabled:[[:space:]]*true[[:space:]]*(#.*)?$'
}

# nerv_repo_enabled ROOT
#
# Reports, via exit status, whether ROOT/.ai-workflow.yml enables NERV.
nerv_repo_enabled() {
  nerv_file_enabled "$1/.ai-workflow.yml"
}

# nerv_legacy_enabled ROOT
#
# Reports, via exit status, whether ROOT has only the pre-rename
# ROOT/.nerv/nerv.yaml (no ROOT/.ai-workflow.yml) and that old file says
# "enabled: true". The old file never activates NERV; this only lets a hook
# point at "nerv migrate".
nerv_legacy_enabled() {
  local root="$1"
  [ -e "${root}/.ai-workflow.yml" ] && return 1
  nerv_file_enabled "${root}/.nerv/nerv.yaml"
}

# nerv_legacy_notice
#
# The one-line notice for a repository that nerv_legacy_enabled accepts.
nerv_legacy_notice() {
  echo 'NERV: .nerv/nerv.yaml is no longer read (configuration now lives in .ai-workflow.yml); run "nerv migrate" to move it. NERV is not active in this session.'
}
