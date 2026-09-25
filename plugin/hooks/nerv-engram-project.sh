#!/usr/bin/env bash
# nerv-engram-project.sh
#
# SessionStart hook for the NERV plugin. Detects which Engram project the
# current repo resolves to and prints guidance so the session (and, inside
# an opted-in NERV repo, the Ikari protocol) knows which project to pass on
# every Engram write. Runs in EVERY session — unlike nerv-session-start.sh,
# the detection line below is NOT gated behind ".nerv/nerv.yaml".
#
# Must never fail the session: always exits 0, errors go to stderr only.

main() {
  local dir plugin_root
  dir="${CLAUDE_PROJECT_DIR:-$PWD}"
  plugin_root="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd 2>/dev/null)}"
  : "$plugin_root" # reserved input, not otherwise used by detection below

  local project_name="" source_label="" determined=0

  # Resolve the repository root first: both the Engram config and the NERV
  # activation file live at the git toplevel, so a session started in a
  # nested subfolder must still find them. Without git (or outside a repo)
  # the session directory is the only root we have.
  local root="$dir"
  local toplevel=""
  if toplevel="$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null)" && [ -n "$toplevel" ]; then
    root="$toplevel"
  fi

  # --- Detection source 1: .engram/config.json (session dir, then repo root) ---
  # The file is repository-controlled input: the extracted name is accepted
  # only when it matches a strict charset (letters, digits, dot, underscore,
  # dash; 1-64 chars, must start alphanumeric) and is not the reserved
  # knowledge-base project "nerv", which no repository may claim for itself.
  local config_file candidate
  for config_file in "${dir}/.engram/config.json" "${root}/.engram/config.json"; do
    [ -f "$config_file" ] || continue
    candidate="$(tr -d '\r' < "$config_file" 2>/dev/null \
      | grep -o '"project_name"[[:space:]]*:[[:space:]]*"[^"]*"' \
      | head -n 1 \
      | sed -E 's/^"project_name"[[:space:]]*:[[:space:]]*"([^"]*)"$/\1/')"
    [ -n "$candidate" ] || continue
    if [ "$candidate" = "nerv" ]; then
      echo "nerv-engram-project: ${config_file} names the reserved knowledge-base project \"nerv\"; ignored" >&2
      break
    fi
    if ! printf '%s' "$candidate" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'; then
      echo "nerv-engram-project: ${config_file} project_name rejected (allowed: [A-Za-z0-9._-], 1-64 chars, alphanumeric first); ignored" >&2
      break
    fi
    project_name="$candidate"
    source_label=".engram/config.json"
    determined=1
    break
  done

  # --- Detection source 2: git toplevel basename ---
  if [ "$determined" -eq 0 ] && [ -n "$toplevel" ]; then
    project_name="$(basename "$toplevel")"
    source_label="git toplevel"
    determined=1
  fi

  if [ "$determined" -eq 1 ]; then
    echo "Engram project: ${project_name} (source: ${source_label})"
  fi

  # --- NERV activation gate (same CRLF-tolerant regex as
  # nerv-session-start.sh), only for the knowledge-base guidance below ---
  local nerv_config="${root}/.nerv/nerv.yaml"
  local nerv_enabled=0
  if [ -f "$nerv_config" ] \
    && tr -d '\r' < "$nerv_config" 2>/dev/null | grep -Eq '^[[:space:]]*enabled:[[:space:]]*true[[:space:]]*(#.*)?$'; then
    nerv_enabled=1
  fi

  if [ "$nerv_enabled" -eq 1 ]; then
    echo 'NERV knowledge base: Engram project "nerv" — read precedents there before deciding, mirror decisions there (see nerv-phase-common.md).'
    if [ "$determined" -eq 0 ]; then
      echo 'Engram project: nerv (source: NERV fallback — the repo resolved no project; pass project: "nerv" on every Engram write)'
    fi
  elif [ "$determined" -eq 0 ]; then
    echo 'Engram project: undetermined — before the first Engram write (mem_save, mem_session_summary, mem_context) ask the user ONE question: general knowledge for the "root" project, or which named project? Then pass project: "<answer>" (with the recovery token when the error returned one). Never pick a project silently.'
  fi

  return 0
}

main || true
exit 0
