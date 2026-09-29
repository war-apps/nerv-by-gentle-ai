#!/usr/bin/env bash
# get-nerv.sh — bootstrap installer for NERV Gentle-AI on Linux and macOS.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/get-nerv.sh | bash
#   wget -qO- https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/get-nerv.sh | bash
#   bash get-nerv.sh [--channel stable|alpha|rc] [--dir <path>] [--no-configure]
#
# Run `get-nerv.sh --help` for the full option list, and see the "Install"
# section of README.md for end-user documentation. tests/get-nerv.test.sh
# covers this file's behaviour with a stubbed PATH and a local fixture repo.
set -euo pipefail

NERV_REPO_URL_DEFAULT="https://github.com/war-apps/nerv-gentle-ai.git"
NERV_API_BASE_DEFAULT="https://api.github.com/repos/war-apps/nerv-gentle-ai"

usage() {
  cat <<'USAGE'
Usage: get-nerv.sh [options]

Bootstrap installer for NERV Gentle-AI: checks out a release channel and
delegates to plugin/tools/install.ps1 (marketplace registration, plugin
cache refresh, skills, and the configuration wizard).

Options:
  --channel <stable|alpha|rc>  Release channel to install (default: stable)
  --dir <path>                 Checkout directory (default: $HOME/.nerv/src)
  --no-configure                Skip the configuration wizard after installing
  --help                        Show this help and exit

Environment fallbacks (used only when the matching flag is not given):
  NERV_CHANNEL        same as --channel
  NERV_HOME           same as --dir
  NERV_NO_CONFIGURE   non-empty skips the wizard, same as --no-configure

Advanced/test hooks:
  NERV_REPO_URL      git remote to clone/fetch from
                     (default: https://github.com/war-apps/nerv-gentle-ai.git)
  NERV_API_BASE      GitHub API base used to resolve releases
                     (default: https://api.github.com/repos/war-apps/nerv-gentle-ai)
  NERV_JSON_PARSER   force the JSON parser ("python3" or "sed"); autodetected
                     otherwise (python3 when available, else a conservative
                     grep/sed extraction — see json_get_string/
                     json_find_prerelease_tag below)
  NERV_TTY_OVERRIDE  "1"/"0" forces the has-a-terminal check that decides
                     whether the configuration wizard can attach to a real
                     console; unset probes /dev/tty for real
  NERV_TTY_DEVICE    device to read the wizard's input from when a terminal
                     is available (default: /dev/tty; tests point this at
                     /dev/null so they never touch a real controlling tty)
USAGE
}

log_err() {
  printf '%s\n' "$*" >&2
}

json_parser() {
  if [ -n "${NERV_JSON_PARSER:-}" ]; then
    printf '%s' "$NERV_JSON_PARSER"
    return 0
  fi
  if command -v python3 >/dev/null 2>&1; then
    printf 'python3'
  else
    printf 'sed'
  fi
}

# json_get_string FILE KEY — prints the string value of a top-level key in a
# single JSON object (the shape of GET /releases/latest). python3 does a
# real parse; the fallback is a conservative extraction of the first
# `"KEY": "value"` occurrence, which is enough for GitHub's flat release
# fields (tag_name never contains an escaped quote).
json_get_string() {
  local file="$1" key="$2"
  case "$(json_parser)" in
    python3)
      python3 - "$file" "$key" <<'PYEOF'
import json, sys
with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)
value = data.get(sys.argv[2])
print(value if value is not None else "")
PYEOF
      ;;
    *)
      local matches first
      matches="$(grep -oE "\"${key}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" "$file" 2>/dev/null)" || matches=""
      first="${matches%%$'\n'*}"
      printf '%s' "$first" | sed -E 's/.*:[[:space:]]*"([^"]*)"/\1/'
      ;;
  esac
}

# json_find_prerelease_tag FILE MARKER — prints the tag_name of the first
# array entry (in array order, i.e. newest-first as GitHub returns releases)
# whose "prerelease" is true and whose tag_name contains MARKER; empty when
# none match. The fallback assumes tag_name and prerelease each appear
# exactly once per release object, in a stable relative order across
# objects — true for GitHub's release-list payload — and zips the two
# extracted lists by position instead of parsing object boundaries.
json_find_prerelease_tag() {
  local file="$1" marker="$2"
  case "$(json_parser)" in
    python3)
      python3 - "$file" "$marker" <<'PYEOF'
import json, sys
with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)
marker = sys.argv[2]
for item in data:
    tag = item.get("tag_name") or ""
    if item.get("prerelease") and marker in tag:
        print(tag)
        break
PYEOF
      ;;
    *)
      local tags prereleases tag pre found
      tags="$(grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' "$file" 2>/dev/null | sed -E 's/.*:[[:space:]]*"([^"]*)"/\1/')" || tags=""
      prereleases="$(grep -oE '"prerelease"[[:space:]]*:[[:space:]]*(true|false)' "$file" 2>/dev/null | sed -E 's/.*:[[:space:]]*(true|false)/\1/')" || prereleases=""
      found=""
      while IFS= read -r tag <&3 && IFS= read -r pre <&4; do
        if [ "$pre" = "true" ]; then
          case "$tag" in
            *"$marker"*)
              found="$tag"
              break
              ;;
          esac
        fi
      done 3<<<"$tags" 4<<<"$prereleases"
      printf '%s' "$found"
      ;;
  esac
}

http_client() {
  if command -v curl >/dev/null 2>&1; then
    printf 'curl'
  elif command -v wget >/dev/null 2>&1; then
    printf 'wget'
  else
    printf ''
  fi
}

fetch_json() {
  local url="$1" outfile="$2" client
  client="$(http_client)"
  case "$client" in
    curl) curl -fsSL "$url" -o "$outfile" ;;
    wget) wget -qO "$outfile" "$url" ;;
    *)
      log_err "curl or wget is required to query the GitHub API."
      exit 1
      ;;
  esac
}

# resolve_ref CHANNEL — prints "KIND\nREF" on stdout (KIND is "tag" or
# "branch"), or exits 1 with a message on stderr when no ref can be
# resolved for the channel.
resolve_ref() {
  local channel="$1" response_file
  response_file="$(mktemp)"

  case "$channel" in
    stable)
      fetch_json "${NERV_API_BASE}/releases/latest" "$response_file"
      local tag
      tag="$(json_get_string "$response_file" tag_name)"
      rm -f "$response_file"
      if [ -z "$tag" ]; then
        log_err "Could not resolve the latest stable release from ${NERV_API_BASE}/releases/latest."
        exit 1
      fi
      printf 'tag\n%s\n' "$tag"
      ;;
    alpha|rc)
      fetch_json "${NERV_API_BASE}/releases?per_page=50" "$response_file"
      local marker tag
      marker="-${channel}."
      tag="$(json_find_prerelease_tag "$response_file" "$marker")"
      rm -f "$response_file"
      if [ -n "$tag" ]; then
        printf 'tag\n%s\n' "$tag"
      elif [ "$channel" = "alpha" ]; then
        printf 'branch\n%s\n' "develop"
      else
        log_err "No rc pre-release published yet."
        exit 1
      fi
      ;;
  esac
}

# checkout_ref DIR REF KIND — clones DIR fresh when it has no .git; otherwise
# fetches REF and checks it out. `git fetch origin <ref>` alone only updates
# FETCH_HEAD, not a local ref, so a tag is fetched into an explicit local
# refs/tags/<ref> (kept in sync with `+`, in case it already exists locally)
# and checked out from there (detached HEAD); a branch is fetched into
# FETCH_HEAD and checked out via `checkout -B <ref> FETCH_HEAD`, which does
# create/reset the local branch. Refuses to touch a DIR that already exists
# and is not a git repository.
checkout_ref() {
  local dir="$1" ref="$2" kind="$3"

  if [ -e "$dir" ] && [ ! -d "${dir}/.git" ]; then
    log_err "${dir} already exists and is not a git repository."
    exit 1
  fi

  if [ -d "${dir}/.git" ]; then
    case "$kind" in
      tag)
        git -C "$dir" fetch --depth 1 origin "+refs/tags/${ref}:refs/tags/${ref}"
        git -C "$dir" checkout --detach "refs/tags/${ref}"
        ;;
      branch)
        git -C "$dir" fetch --depth 1 origin "$ref"
        git -C "$dir" checkout -B "$ref" FETCH_HEAD
        ;;
    esac
  else
    mkdir -p "$(dirname "$dir")"
    git clone --depth 1 --branch "$ref" "$NERV_REPO_URL" "$dir"
  fi
}

has_tty() {
  if [ -n "${NERV_TTY_OVERRIDE:-}" ]; then
    [ "$NERV_TTY_OVERRIDE" = "1" ]
    return $?
  fi
  [ -r /dev/tty ]
}

tty_device() {
  printf '%s' "${NERV_TTY_DEVICE:-/dev/tty}"
}

check_prereqs() {
  if ! command -v git >/dev/null 2>&1; then
    log_err "git is required but was not found on PATH."
    log_err "Install it: https://git-scm.com/downloads"
    exit 1
  fi

  if ! command -v pwsh >/dev/null 2>&1; then
    log_err "pwsh (PowerShell 7+) is required but was not found on PATH."
    case "$(uname -s 2>/dev/null || true)" in
      Darwin) log_err "Install it with: brew install --cask powershell" ;;
      Linux)
        if [ -f /etc/debian_version ]; then
          log_err "Install it: https://learn.microsoft.com/powershell/scripting/install/install-ubuntu"
        else
          log_err "Install it: https://aka.ms/install-powershell"
        fi
        ;;
      *) log_err "Install it: https://aka.ms/install-powershell" ;;
    esac
    exit 1
  fi

  local pwsh_major
  # shellcheck disable=SC2016 # literal PowerShell expression, not a bash var
  pwsh_major="$(pwsh -NoProfile -Command '$PSVersionTable.PSVersion.Major' 2>/dev/null)" || pwsh_major=""
  pwsh_major="$(printf '%s' "$pwsh_major" | tr -d '[:space:]')"
  case "$pwsh_major" in
    ''|*[!0-9]*)
      log_err "Could not determine pwsh's version (got: '${pwsh_major}')."
      log_err "pwsh 7 or newer is required: https://aka.ms/install-powershell"
      exit 1
      ;;
  esac
  if [ "$pwsh_major" -lt 7 ]; then
    log_err "pwsh 7 or newer is required (found major version ${pwsh_major})."
    log_err "Install it: https://aka.ms/install-powershell"
    exit 1
  fi

  if ! command -v claude >/dev/null 2>&1; then
    log_err "claude (Claude Code CLI) is required but was not found on PATH."
    log_err "Install it: https://docs.claude.com/en/docs/claude-code/setup"
    exit 1
  fi

  if [ -z "$(http_client)" ]; then
    log_err "curl or wget is required to query the GitHub API."
    exit 1
  fi
}

main() {
  local channel="${NERV_CHANNEL:-stable}"
  local dir="${NERV_HOME:-${HOME:-}/.nerv/src}"
  local configure=1
  if [ -n "${NERV_NO_CONFIGURE:-}" ]; then
    configure=0
  fi

  while [ $# -gt 0 ]; do
    case "$1" in
      --channel)
        if [ $# -lt 2 ]; then
          log_err "--channel requires a value."
          usage >&2
          exit 2
        fi
        channel="$2"
        shift 2
        ;;
      --dir)
        if [ $# -lt 2 ]; then
          log_err "--dir requires a value."
          usage >&2
          exit 2
        fi
        dir="$2"
        shift 2
        ;;
      --no-configure)
        configure=0
        shift
        ;;
      --help)
        usage
        exit 0
        ;;
      *)
        log_err "Unknown option: $1"
        usage >&2
        exit 2
        ;;
    esac
  done

  case "$channel" in
    stable|alpha|rc) ;;
    *)
      log_err "Unknown channel: $channel"
      usage >&2
      exit 2
      ;;
  esac

  check_prereqs

  NERV_REPO_URL="${NERV_REPO_URL:-$NERV_REPO_URL_DEFAULT}"
  NERV_API_BASE="${NERV_API_BASE:-$NERV_API_BASE_DEFAULT}"

  local resolved kind ref
  resolved="$(resolve_ref "$channel")"
  kind="${resolved%%$'\n'*}"
  ref="${resolved#*$'\n'}"

  echo "Resolved channel '${channel}' to ${kind} '${ref}'."

  checkout_ref "$dir" "$ref" "$kind"

  local pwsh_args
  pwsh_args=(-NoProfile -File "${dir}/plugin/tools/install.ps1" -RefreshCache -Skills)
  local use_tty=0

  if [ "$configure" -eq 1 ]; then
    if has_tty; then
      pwsh_args+=(-Configure)
      use_tty=1
    else
      echo "No interactive terminal available; skipping the configuration wizard."
      echo "Run it later with: pwsh ${dir}/plugin/tools/configure.ps1"
    fi
  fi

  echo "Running: pwsh ${pwsh_args[*]}"
  if [ "$use_tty" -eq 1 ]; then
    pwsh "${pwsh_args[@]}" <"$(tty_device)"
  else
    pwsh "${pwsh_args[@]}"
  fi
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
