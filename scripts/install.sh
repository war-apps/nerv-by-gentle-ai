#!/usr/bin/env bash
# scripts/install.sh — bootstrap installer for the nerv binary on Linux and
# macOS.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/scripts/install.sh | bash
#   wget -qO- https://raw.githubusercontent.com/war-apps/nerv-gentle-ai/main/scripts/install.sh | bash
#
# Downloads the nerv release archive matching this platform from the
# requested channel, verifies its checksum, installs the "nerv" binary,
# then runs "nerv install" (the interactive setup wizard runs at the end
# when a terminal is attached). See tests/install-sh.test.sh for the suite
# that exercises this file's behaviour against fixtures. See README.md's
# "Install" section for end-user documentation.
#
# Options (environment variables only — this script is normally piped into
# bash, so there is no argv to parse):
#   NERV_CHANNEL       stable | alpha | rc (default: stable)
#   NERV_INSTALL_DIR   install directory (default: $HOME/.local/bin)
#   NERV_NO_INSTALL    non-empty: download and install the binary only,
#                       skip running "nerv install" afterwards
#   NERV_VERSION       explicit release tag (e.g. v1.2.3) — overrides
#                       channel resolution entirely, no API call is made
#
# Test seams (tests/install-sh.test.sh only, never needed by end users):
#   NERV_API_BASE       GitHub API base used to resolve releases
#                        (default: https://api.github.com/repos/war-apps/nerv-gentle-ai)
#   NERV_DOWNLOAD_BASE  release download base
#                        (default: https://github.com/war-apps/nerv-gentle-ai/releases/download)
#   NERV_OS / NERV_ARCH  override the detected platform with an
#                        already-normalized value (linux|darwin,
#                        amd64|arm64) instead of probing "uname -s"/"-m"
#   NERV_TTY_OVERRIDE    "1"/"0" forces the has-a-terminal check that
#                        decides whether "nerv install" can attach the
#                        setup wizard to a real console; unset probes
#                        /dev/tty for real
#   NERV_TTY_DEVICE      device to reattach stdin from when a terminal is
#                        available (default: /dev/tty; tests point this at
#                        /dev/null so they never touch a real controlling
#                        tty)
set -euo pipefail

NERV_API_BASE_DEFAULT="https://api.github.com/repos/war-apps/nerv-gentle-ai"
NERV_DOWNLOAD_BASE_DEFAULT="https://github.com/war-apps/nerv-gentle-ai/releases/download"

# Set by main() and read by its EXIT trap. Deliberately a global, not a
# local of main(): a local referenced by a trap that ends up firing inside
# a subshell forked by a deeper call (e.g. verify_checksum's "exit 1")
# reads back as unbound under "set -u" even though the same frame is still
# active on the call stack.
NERV_TMP_DIR=""

log_err() {
  printf '%s\n' "$*" >&2
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

# fetch URL OUTFILE — downloads URL into OUTFILE with curl or wget. main
# always runs check_prereqs (the single "curl or wget is required" check)
# before any call path that reaches fetch.
fetch() {
  local url="$1" outfile="$2" client
  client="$(http_client)"
  case "$client" in
    curl) curl -fsSL "$url" -o "$outfile" ;;
    wget) wget -qO "$outfile" "$url" ;;
  esac
}

# detect_platform — sets the globals NERV_DETECTED_OS / NERV_DETECTED_ARCH
# from "uname -s"/"uname -m" (mapped to goreleaser's linux|darwin and
# amd64|arm64), or from the already-normalized NERV_OS/NERV_ARCH test
# overrides when set. Exits 1 with a go-install hint for a platform
# goreleaser does not build.
detect_platform() {
  if [ -n "${NERV_OS:-}" ]; then
    NERV_DETECTED_OS="$NERV_OS"
  else
    local uname_os
    uname_os="$(uname -s)"
    case "$uname_os" in
      Linux) NERV_DETECTED_OS="linux" ;;
      Darwin) NERV_DETECTED_OS="darwin" ;;
      *) NERV_DETECTED_OS="$uname_os" ;;
    esac
  fi

  if [ -n "${NERV_ARCH:-}" ]; then
    NERV_DETECTED_ARCH="$NERV_ARCH"
  else
    local uname_arch
    uname_arch="$(uname -m)"
    case "$uname_arch" in
      x86_64 | amd64) NERV_DETECTED_ARCH="amd64" ;;
      arm64 | aarch64) NERV_DETECTED_ARCH="arm64" ;;
      *) NERV_DETECTED_ARCH="$uname_arch" ;;
    esac
  fi

  case "$NERV_DETECTED_OS" in
    linux | darwin) ;;
    *)
      log_err "Unsupported OS: ${NERV_DETECTED_OS}. Only Linux and macOS have prebuilt nerv binaries."
      log_err "Install with: go install github.com/war-apps/nerv-gentle-ai/cmd/nerv@latest"
      exit 1
      ;;
  esac

  case "$NERV_DETECTED_ARCH" in
    amd64 | arm64) ;;
    *)
      log_err "Unsupported architecture: ${NERV_DETECTED_ARCH}. Only amd64 and arm64 have prebuilt nerv binaries."
      log_err "Install with: go install github.com/war-apps/nerv-gentle-ai/cmd/nerv@latest"
      exit 1
      ;;
  esac
}

check_prereqs() {
  if ! command -v claude >/dev/null 2>&1; then
    log_err "claude (Claude Code CLI) is required but was not found on PATH."
    log_err "Install it: https://docs.claude.com/en/docs/claude-code/setup"
    exit 1
  fi

  if [ -z "$(http_client)" ]; then
    log_err "curl or wget is required to install nerv."
    exit 1
  fi
}

# json_get_string FILE KEY — prints the string value of the first top-level
# "KEY": "value" occurrence in FILE. A conservative extraction, sufficient
# for GitHub's flat release fields (tag_name never contains an escaped
# quote).
json_get_string() {
  local file="$1" key="$2" matches first
  matches="$(grep -oE "\"${key}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" "$file" 2>/dev/null)" || matches=""
  first="${matches%%$'\n'*}"
  printf '%s' "$first" | sed -E 's/.*:[[:space:]]*"([^"]*)"/\1/'
}

# json_find_prerelease_tag FILE MARKER — prints the tag_name of the first
# array entry (array order = newest-first, as GitHub returns releases)
# whose "prerelease" is true and whose tag_name contains MARKER; empty when
# none match. Zips the two extracted lists by position, which holds because
# tag_name and prerelease each appear exactly once per release object, in a
# stable relative order across objects.
json_find_prerelease_tag() {
  local file="$1" marker="$2" tags prereleases tag pre found
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
}

# resolve_tag CHANNEL — prints the release tag to install for CHANNEL, or
# exits 1 with a message on stderr when none can be resolved. Binaries
# only exist for published releases, so unlike a source checkout there is
# no branch fallback for an empty alpha channel.
resolve_tag() {
  local channel="$1" response_file tag

  if [ -n "${NERV_VERSION:-}" ]; then
    printf '%s' "$NERV_VERSION"
    return 0
  fi

  response_file="$(mktemp)"

  # channel is one of stable|alpha|rc here: main validates it (the single
  # "Unknown channel" check) before ever calling resolve_tag.
  case "$channel" in
    stable)
      fetch "${NERV_API_BASE}/releases/latest" "$response_file"
      tag="$(json_get_string "$response_file" tag_name)"
      rm -f "$response_file"
      if [ -z "$tag" ]; then
        log_err "Could not resolve the latest stable release from ${NERV_API_BASE}/releases/latest."
        exit 1
      fi
      printf '%s' "$tag"
      ;;
    alpha | rc)
      fetch "${NERV_API_BASE}/releases?per_page=50" "$response_file"
      tag="$(json_find_prerelease_tag "$response_file" "-${channel}.")"
      rm -f "$response_file"
      if [ -z "$tag" ]; then
        log_err "No ${channel} pre-release published yet."
        exit 1
      fi
      printf '%s' "$tag"
      ;;
  esac
}

# download_release TAG OS ARCH DEST_DIR — downloads the archive and
# checksums.txt for TAG/OS/ARCH into DEST_DIR and prints the archive's
# filename.
download_release() {
  local tag="$1" os="$2" arch="$3" dest_dir="$4"
  local version_no_v="${tag#v}"
  local archive="nerv_${version_no_v}_${os}_${arch}.tar.gz"
  local base="${NERV_DOWNLOAD_BASE}/${tag}"

  fetch "${base}/${archive}" "${dest_dir}/${archive}"
  fetch "${base}/checksums.txt" "${dest_dir}/checksums.txt"

  printf '%s' "$archive"
}

# verify_checksum DEST_DIR ARCHIVE — verifies ARCHIVE against its entry in
# DEST_DIR/checksums.txt, restricted to that one entry (checksums.txt lists
# every platform's archive; only ARCHIVE was downloaded). Exits 1 on any
# failure: no sha256 tool, no checksums.txt, no matching entry, or a
# mismatch.
verify_checksum() {
  local dest_dir="$1" archive="$2"
  (
    cd "$dest_dir"

    local checksum_cmd
    if command -v sha256sum >/dev/null 2>&1; then
      checksum_cmd=(sha256sum)
    elif command -v shasum >/dev/null 2>&1; then
      checksum_cmd=(shasum -a 256)
    else
      log_err "sha256sum or shasum is required to verify the download."
      exit 1
    fi

    if [ ! -f checksums.txt ]; then
      log_err "checksums.txt was not downloaded."
      exit 1
    fi

    # sha256sum's filename field can carry a leading "*" (binary-mode
    # marker, printed on platforms that distinguish binary/text i/o); the
    # archive was downloaded as plain bytes either way, so it is stripped
    # before comparing.
    awk -v f="$archive" '{name=$2; sub(/^\*/, "", name); if (name == f) print}' checksums.txt >checksums.selected.txt
    if [ ! -s checksums.selected.txt ]; then
      log_err "${archive} was not found in checksums.txt."
      exit 1
    fi

    if ! "${checksum_cmd[@]}" -c checksums.selected.txt >/dev/null 2>&1; then
      log_err "Checksum verification failed for ${archive}."
      exit 1
    fi
  )
}

# extract_and_install DEST_DIR ARCHIVE INSTALL_DIR — extracts the nerv
# binary from ARCHIVE and installs it at INSTALL_DIR/nerv with mode 0755,
# creating INSTALL_DIR if needed.
extract_and_install() {
  local dest_dir="$1" archive="$2" install_dir="$3"

  tar -xzf "${dest_dir}/${archive}" -C "$dest_dir"
  if [ ! -f "${dest_dir}/nerv" ]; then
    log_err "nerv binary not found inside ${archive}."
    exit 1
  fi

  mkdir -p "$install_dir"
  install -m 0755 "${dest_dir}/nerv" "${install_dir}/nerv"
}

warn_if_not_on_path() {
  local install_dir="$1"
  case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *)
      log_err "${install_dir} is not on your PATH."
      log_err "Add this to your shell profile: export PATH=\"\$PATH:${install_dir}\""
      ;;
  esac
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

# run_nerv_install NERV_BIN — runs "nerv install", reattaching stdin from a
# real terminal when one is available so the closing setup wizard can run;
# otherwise runs it with --no-configure and prints how to finish setup
# later. Returns nerv install's own exit code.
run_nerv_install() {
  local nerv_bin="$1" rc=0

  if has_tty; then
    echo "Running: ${nerv_bin} install"
    "$nerv_bin" install <"$(tty_device)" || rc=$?
  else
    echo "No interactive terminal available; running: ${nerv_bin} install --no-configure"
    "$nerv_bin" install --no-configure || rc=$?
    echo "Run '${nerv_bin} configure' later to finish setup."
  fi

  return "$rc"
}

main() {
  local channel="${NERV_CHANNEL:-stable}"
  local install_dir="${NERV_INSTALL_DIR:-${HOME:-}/.local/bin}"
  local no_install="${NERV_NO_INSTALL:-}"

  case "$channel" in
    stable | alpha | rc) ;;
    *)
      log_err "Unknown channel: ${channel}. Use: stable, alpha, or rc."
      exit 1
      ;;
  esac

  detect_platform
  check_prereqs

  NERV_API_BASE="${NERV_API_BASE:-$NERV_API_BASE_DEFAULT}"
  NERV_DOWNLOAD_BASE="${NERV_DOWNLOAD_BASE:-$NERV_DOWNLOAD_BASE_DEFAULT}"

  local tag
  tag="$(resolve_tag "$channel")"
  echo "Resolved channel '${channel}' to ${tag}."

  NERV_TMP_DIR="$(mktemp -d)"
  trap 'rm -rf "$NERV_TMP_DIR"' EXIT

  local archive
  archive="$(download_release "$tag" "$NERV_DETECTED_OS" "$NERV_DETECTED_ARCH" "$NERV_TMP_DIR")"
  verify_checksum "$NERV_TMP_DIR" "$archive"
  echo "Checksum verified for ${archive}."

  extract_and_install "$NERV_TMP_DIR" "$archive" "$install_dir"
  echo "Installed nerv to ${install_dir}/nerv."

  warn_if_not_on_path "$install_dir"

  if [ -n "$no_install" ]; then
    echo "NERV_NO_INSTALL set; skipping 'nerv install'. Run '${install_dir}/nerv install' later."
    return 0
  fi

  run_nerv_install "${install_dir}/nerv"
}

if [ -z "${BASH_SOURCE[0]:-}" ] || [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
