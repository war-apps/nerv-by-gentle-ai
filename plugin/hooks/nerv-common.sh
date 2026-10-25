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
# Reports, via exit status, whether ROOT/.ai-workflow/config.yaml enables
# NERV and no previous ROOT/.ai-workflow.yml is left: a leftover previous
# file is never read, and never silently ignored either (see
# nerv_legacy_enabled).
nerv_repo_enabled() {
  [ -e "$1/.ai-workflow.yml" ] && return 1
  nerv_file_enabled "$1/.ai-workflow/config.yaml"
}

# nerv_legacy_enabled ROOT
#
# Reports, via exit status, whether ROOT opted in only through a file nerv
# no longer reads, and prints that file's path relative to ROOT:
#   - the previous ROOT/.ai-workflow.yml, when it says "enabled: true" or
#     sits next to a ROOT/.ai-workflow/config.yaml that does;
#   - otherwise the 4.2.0 ROOT/.nerv/nerv.yaml, when it says
#     "enabled: true" and ROOT/.ai-workflow/config.yaml is absent.
# The old files never activate NERV; this only lets a hook point at
# "nerv migrate".
nerv_legacy_enabled() {
  local root="$1"
  if [ -e "${root}/.ai-workflow.yml" ]; then
    if nerv_file_enabled "${root}/.ai-workflow.yml" || nerv_file_enabled "${root}/.ai-workflow/config.yaml"; then
      echo ".ai-workflow.yml"
      return 0
    fi
    return 1
  fi
  [ -e "${root}/.ai-workflow/config.yaml" ] && return 1
  nerv_file_enabled "${root}/.nerv/nerv.yaml" || return 1
  echo ".nerv/nerv.yaml"
}

# nerv_legacy_notice OLD_FILE
#
# The one-line notice for the file nerv_legacy_enabled printed.
nerv_legacy_notice() {
  echo "NERV: $1 is no longer read (configuration now lives in .ai-workflow/config.yaml); run \"nerv migrate\" to move it. NERV is not active in this session."
}
