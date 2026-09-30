#!/usr/bin/env bash
# install-sh.test.sh
#
# Bash/POSIX-friendly assertions for scripts/install.sh (the curl|bash /
# wget|bash bootstrap installer for the "nerv" binary). No external deps
# beyond coreutils, tar and sha256sum.
#
# Run with:
#   bash tests/install-sh.test.sh
#
# Exits 0 if every case passes, 1 if any case fails.
# Prints one "PASS <case>" / "FAIL <case>" / "SKIP <case>" line per case.
#
# Test seams introduced by scripts/install.sh that this suite relies on:
#   NERV_API_BASE       GitHub API base (matched by exact prefix in the
#                        curl stub)
#   NERV_DOWNLOAD_BASE  release download base (matched by exact prefix in
#                        the curl stub)
#   NERV_OS / NERV_ARCH override "uname -s" / "uname -m" detection
#   NERV_TTY_OVERRIDE   "1"/"0" forces has_tty() instead of probing
#                       /dev/tty
#   NERV_TTY_DEVICE     device to reattach the wizard's stdin from when a
#                       tty is available (tests use /dev/null; never a
#                       real tty)
# Plus stub-only environment variables read by the stub executables below:
#   NERV_FIXTURES, NERV_STUB_CURL_LOG, NERV_STUB_LOG, NERV_STUB_EXIT,
#   NERV_STUB_CLAUDE_EXIT

set -u
set -o pipefail

self_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${self_dir}/.." && pwd)"
install_script="${repo_root}/scripts/install.sh"

pass_count=0
fail_count=0
skip_count=0

report() {
  local case_name="$1" ok="$2" detail="${3:-}"
  if [ "$ok" -eq 0 ]; then
    echo "PASS ${case_name}"
    pass_count=$((pass_count + 1))
  else
    echo "FAIL ${case_name}${detail:+ - ${detail}}"
    fail_count=$((fail_count + 1))
  fi
}

skip() {
  local case_name="$1" reason="${2:-}"
  echo "SKIP ${case_name}${reason:+ - ${reason}}"
  skip_count=$((skip_count + 1))
}

# line_has_all LINE PATTERN... — true only when LINE contains every PATTERN
# as a literal substring. Avoids piping into grep -q (EPIPE risk under
# pipefail); operates on an in-memory string via plain `case`.
line_has_all() {
  local line="$1"
  shift
  local pattern
  for pattern in "$@"; do
    case "$line" in
      *"$pattern"*) : ;;
      *) return 1 ;;
    esac
  done
  return 0
}

# remove_dir_from_path PATH_VALUE DIR — prints PATH_VALUE with every
# occurrence of DIR removed (a PATH can list the same directory twice).
remove_dir_from_path() {
  local path_val="$1" dir="$2" entry result=""
  local old_ifs="$IFS"
  IFS=':'
  for entry in $path_val; do
    if [ "$entry" != "$dir" ]; then
      if [ -z "$result" ]; then
        result="$entry"
      else
        result="${result}:${entry}"
      fi
    fi
  done
  IFS="$old_ifs"
  printf '%s' "$result"
}

# strip_dir_of PATH_VALUE BIN_NAME — prints PATH_VALUE with every directory
# that resolves BIN_NAME (via `command -v`) removed, so BIN_NAME becomes
# fully unresolvable, while every unrelated directory stays available.
# Loops because a machine can expose the same tool from more than one PATH
# entry. No-op when BIN_NAME is not found at all.
strip_dir_of() {
  local path_val="$1" bin_name="$2" bin_path dir
  while :; do
    bin_path="$(PATH="$path_val" command -v "$bin_name" 2>/dev/null)" || bin_path=""
    [ -z "$bin_path" ] && break
    dir="$(dirname "$bin_path")"
    path_val="$(remove_dir_from_path "$path_val" "$dir")"
  done
  printf '%s' "$path_val"
}

# run_install_sh OUT_FILE ERR_FILE PATH_VALUE — runs scripts/install.sh with
# PATH replaced by PATH_VALUE, HOME pointed at a scratch directory (so a
# missing NERV_INSTALL_DIR override could never touch the real home), and
# every currently-exported NERV_*/stub variable inherited as-is. Returns
# install.sh's own exit code.
run_install_sh() {
  local out_file="$1" err_file="$2" path_val="$3"
  PATH="$path_val" HOME="$work_dir/unused-home" bash "$install_script" >"$out_file" 2>"$err_file"
}

reset_test_env() {
  unset NERV_CHANNEL NERV_INSTALL_DIR NERV_NO_INSTALL NERV_VERSION \
    NERV_API_BASE NERV_DOWNLOAD_BASE NERV_OS NERV_ARCH \
    NERV_TTY_OVERRIDE NERV_TTY_DEVICE NERV_FIXTURES \
    NERV_STUB_CURL_LOG NERV_STUB_LOG NERV_STUB_EXIT NERV_STUB_CLAUDE_EXIT \
    2>/dev/null || true
}

write_claude_stub() {
  cat >"$1" <<'STUB'
#!/usr/bin/env bash
exit "${NERV_STUB_CLAUDE_EXIT:-0}"
STUB
  chmod +x "$1"
}

# write_curl_stub PATH — a curl replacement that serves fixture files by
# matching the requested URL against NERV_API_BASE / NERV_DOWNLOAD_BASE
# prefixes (both read from the environment, exported by the caller), and
# logs every requested URL to NERV_STUB_CURL_LOG.
write_curl_stub() {
  cat >"$1" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
: "${NERV_FIXTURES:?NERV_FIXTURES not set}"
# Not every case inspects the requested-URL log, so this is a soft default
# rather than a hard requirement like NERV_FIXTURES above.
: "${NERV_STUB_CURL_LOG:=/dev/null}"
url=""
outfile=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-o" ]; then
    outfile="$arg"
    prev=""
    continue
  fi
  case "$arg" in
    -o) prev="-o" ;;
    -*) ;;
    *) url="$arg" ;;
  esac
done
printf '%s\n' "$url" >>"$NERV_STUB_CURL_LOG"

api_base="${NERV_API_BASE:-}"
download_base="${NERV_DOWNLOAD_BASE:-}"

# The download base is itself "${api_base}/releases/download", so it must be
# matched before the generic "${api_base}/releases"* case below — otherwise
# every download URL would also match that broader releases-list pattern and
# would be served releases.json instead of the requested archive/checksums.
case "$url" in
  "${download_base}/"*)
    rel="${url#"${download_base}/"}"
    if [ -f "${NERV_FIXTURES}/downloads/${rel}" ]; then
      cp "${NERV_FIXTURES}/downloads/${rel}" "$outfile"
    else
      : >"$outfile"
    fi
    ;;
  "${api_base}/releases/latest")
    cp "${NERV_FIXTURES}/latest.json" "$outfile"
    ;;
  "${api_base}/releases"*)
    cp "${NERV_FIXTURES}/releases.json" "$outfile"
    ;;
  *)
    : >"$outfile"
    ;;
esac
exit 0
STUB
  chmod +x "$1"
}

# build_archive_fixture FIXTURES_DIR TAG VERSION_NO_V OS ARCH — writes a
# valid nerv_<version>_<os>_<arch>.tar.gz (containing a fake "nerv" shell
# script that logs its invocation to $NERV_STUB_LOG and exits
# $NERV_STUB_EXIT, both read at runtime) plus a matching checksums.txt
# under FIXTURES_DIR/downloads/TAG/.
build_archive_fixture() {
  local fixtures_dir="$1" tag="$2" version_no_v="$3" os="$4" arch="$5"
  local out_dir="${fixtures_dir}/downloads/${tag}"
  mkdir -p "$out_dir"
  local work
  work="$(mktemp -d)"
  cat >"${work}/nerv" <<'STUB'
#!/usr/bin/env bash
: "${NERV_STUB_LOG:?NERV_STUB_LOG not set}"
printf '%s\n' "$*" >>"$NERV_STUB_LOG"
exit "${NERV_STUB_EXIT:-0}"
STUB
  chmod +x "${work}/nerv"
  local archive="nerv_${version_no_v}_${os}_${arch}.tar.gz"
  tar -C "$work" -czf "${out_dir}/${archive}" nerv
  (cd "$out_dir" && sha256sum "$archive" >checksums.txt)
  rm -rf "$work"
  printf '%s' "$archive"
}

# corrupt_checksum FIXTURES_DIR TAG ARCHIVE — rewrites checksums.txt for
# TAG so it still names ARCHIVE but with a checksum that can never match.
corrupt_checksum() {
  local fixtures_dir="$1" tag="$2" archive="$3"
  local zeros
  zeros="$(printf '0%.0s' $(seq 1 64))"
  printf '%s  %s\n' "$zeros" "$archive" >"${fixtures_dir}/downloads/${tag}/checksums.txt"
}

TEST_API_BASE="https://fixture.test/repos/war-apps/nerv-gentle-ai"
TEST_DOWNLOAD_BASE="https://fixture.test/repos/war-apps/nerv-gentle-ai/releases/download"
TEST_OS="linux"
TEST_ARCH="amd64"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

# --- Shared fixtures: GitHub API payloads reused across cases ---
fixtures_common="$work_dir/fixtures-common"
mkdir -p "$fixtures_common"
cat >"$fixtures_common/latest.json" <<'JSON'
{
  "tag_name": "v0.1.0",
  "prerelease": false,
  "name": "v0.1.0"
}
JSON
cat >"$fixtures_common/releases.json" <<'JSON'
[
  {
    "tag_name": "v0.2.0-rc.1",
    "prerelease": true,
    "name": "v0.2.0-rc.1"
  },
  {
    "tag_name": "v0.2.0-alpha.1",
    "prerelease": true,
    "name": "v0.2.0-alpha.1"
  },
  {
    "tag_name": "v0.1.0",
    "prerelease": false,
    "name": "v0.1.0"
  }
]
JSON

fixtures_empty="$work_dir/fixtures-empty"
mkdir -p "$fixtures_empty"
printf '[]\n' >"$fixtures_empty/releases.json"
cp "$fixtures_common/latest.json" "$fixtures_empty/latest.json"

stable_archive="$(build_archive_fixture "$fixtures_common" "v0.1.0" "0.1.0" "$TEST_OS" "$TEST_ARCH")"
alpha_archive="$(build_archive_fixture "$fixtures_common" "v0.2.0-alpha.1" "0.2.0-alpha.1" "$TEST_OS" "$TEST_ARCH")"
rc_archive="$(build_archive_fixture "$fixtures_common" "v0.2.0-rc.1" "0.2.0-rc.1" "$TEST_OS" "$TEST_ARCH")"
version_override_archive="$(build_archive_fixture "$fixtures_common" "v9.9.9" "9.9.9" "$TEST_OS" "$TEST_ARCH")"

fixtures_bad_checksum="$work_dir/fixtures-bad-checksum"
mkdir -p "$fixtures_bad_checksum"
cp "$fixtures_common/latest.json" "$fixtures_bad_checksum/latest.json"
bad_archive="$(build_archive_fixture "$fixtures_bad_checksum" "v0.1.0" "0.1.0" "$TEST_OS" "$TEST_ARCH")"
corrupt_checksum "$fixtures_bad_checksum" "v0.1.0" "$bad_archive"

# --- Stub executables ---
stub_dir_full="$work_dir/stub-full"
mkdir -p "$stub_dir_full"
write_claude_stub "$stub_dir_full/claude"
write_curl_stub "$stub_dir_full/curl"

# --- PATH variants derived from this test's own (real) PATH ---
original_path="$PATH"
path_no_claude="$(strip_dir_of "$original_path" claude)"
path_base_no_tools="$(strip_dir_of "$(strip_dir_of "$original_path" claude)" curl)"
path_normal="${stub_dir_full}:${path_base_no_tools}"

# =============================================================================
# Case: stable resolves v0.1.0 and installs it
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_NO_INSTALL=1
curl_log="$(mktemp)"
export NERV_STUB_CURL_LOG="$curl_log"
install_dir="$work_dir/install-stable"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 0 ] && [ -x "${install_dir}/nerv" ]; then
  report "stable-resolves-and-installs-v0.1.0" 0
else
  report "stable-resolves-and-installs-v0.1.0" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err" "$curl_log"

# =============================================================================
# Case: alpha resolves v0.2.0-alpha.1
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_CHANNEL="alpha"
export NERV_NO_INSTALL=1
curl_log="$(mktemp)"
export NERV_STUB_CURL_LOG="$curl_log"
install_dir="$work_dir/install-alpha"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 0 ] && [ -x "${install_dir}/nerv" ]; then
  report "alpha-resolves-v0.2.0-alpha.1" 0
else
  report "alpha-resolves-v0.2.0-alpha.1" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err" "$curl_log"

# =============================================================================
# Case: rc resolves v0.2.0-rc.1
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_CHANNEL="rc"
export NERV_NO_INSTALL=1
curl_log="$(mktemp)"
export NERV_STUB_CURL_LOG="$curl_log"
install_dir="$work_dir/install-rc"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 0 ] && [ -x "${install_dir}/nerv" ]; then
  report "rc-resolves-v0.2.0-rc.1" 0
else
  report "rc-resolves-v0.2.0-rc.1" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err" "$curl_log"

# =============================================================================
# Case: alpha with no matching pre-release -> exit 1, no branch fallback
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_empty"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_CHANNEL="alpha"
export NERV_NO_INSTALL=1
curl_log="$(mktemp)"
export NERV_STUB_CURL_LOG="$curl_log"
install_dir="$work_dir/install-alpha-none"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -qi 'no alpha' "$err" && [ ! -e "${install_dir}/nerv" ]; then
  report "alpha-none-exit-1" 0
else
  report "alpha-none-exit-1" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err" "$curl_log"

# =============================================================================
# Case: rc with no matching pre-release -> exit 1
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_empty"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_CHANNEL="rc"
export NERV_NO_INSTALL=1
curl_log="$(mktemp)"
export NERV_STUB_CURL_LOG="$curl_log"
install_dir="$work_dir/install-rc-none"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -qi 'no rc' "$err" && [ ! -e "${install_dir}/nerv" ]; then
  report "rc-none-exit-1" 0
else
  report "rc-none-exit-1" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err" "$curl_log"

# =============================================================================
# Case: NERV_VERSION overrides channel resolution — no API calls made
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_VERSION="v9.9.9"
export NERV_NO_INSTALL=1
curl_log="$(mktemp)"
export NERV_STUB_CURL_LOG="$curl_log"
install_dir="$work_dir/install-version-override"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
# TEST_DOWNLOAD_BASE is itself "${TEST_API_BASE}/releases/download", so a
# plain substring/prefix match against TEST_API_BASE would also match every
# archive/checksums download URL and could never detect "no API call was
# made". Match only the two actual channel-resolution endpoints instead.
api_called=0
while IFS= read -r logged_url; do
  case "$logged_url" in
    "${TEST_API_BASE}/releases/latest" | "${TEST_API_BASE}/releases"\?*)
      api_called=1
      break
      ;;
  esac
done <"$curl_log"
if [ "$exit_code" -eq 0 ] && [ -x "${install_dir}/nerv" ] && [ "$api_called" -eq 0 ]; then
  report "version-override-skips-channel-resolution" 0
else
  report "version-override-skips-channel-resolution" 1 "exit=${exit_code} api_called=${api_called}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err" "$curl_log"

# =============================================================================
# Case: checksum mismatch -> exit 1, installs nothing
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_bad_checksum"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_NO_INSTALL=1
curl_log="$(mktemp)"
export NERV_STUB_CURL_LOG="$curl_log"
install_dir="$work_dir/install-bad-checksum"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -qi 'checksum' "$err" && [ ! -e "${install_dir}/nerv" ]; then
  report "checksum-mismatch-exit-1-installs-nothing" 0
else
  report "checksum-mismatch-exit-1-installs-nothing" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err" "$curl_log"

# =============================================================================
# Case: install dir is created (nested path) with mode 0755
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_NO_INSTALL=1
install_dir="$work_dir/nested/install/dir"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
mode_ok=1
if command -v stat >/dev/null 2>&1 && stat -c '%a' "${install_dir}/nerv" >/dev/null 2>&1; then
  mode="$(stat -c '%a' "${install_dir}/nerv" 2>/dev/null)" || mode=""
  [ "$mode" = "755" ] && mode_ok=0
else
  skip "install-dir-mode-0755" "stat -c unavailable on this platform"
  mode_ok=0
fi
if [ "$exit_code" -eq 0 ] && [ -d "$install_dir" ] && [ -x "${install_dir}/nerv" ] && [ "$mode_ok" -eq 0 ]; then
  report "install-dir-created-and-mode-0755" 0
else
  report "install-dir-created-and-mode-0755" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err"

# =============================================================================
# Case: install dir not on PATH -> warning printed
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_NO_INSTALL=1
install_dir="$work_dir/install-not-on-path"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 0 ] && grep -qi 'not on your PATH' "$err"; then
  report "path-warning-when-install-dir-not-on-path" 0
else
  report "path-warning-when-install-dir-not-on-path" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err"

# =============================================================================
# Case: install dir already on PATH -> no warning
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_NO_INSTALL=1
install_dir="$work_dir/install-on-path"
export NERV_INSTALL_DIR="$install_dir"
mkdir -p "$install_dir"
path_with_install_dir="${install_dir}:${path_normal}"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_with_install_dir"
exit_code=$?
if [ "$exit_code" -eq 0 ] && ! grep -qi 'not on your PATH' "$err"; then
  report "no-path-warning-when-install-dir-already-on-path" 0
else
  report "no-path-warning-when-install-dir-already-on-path" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err"

# =============================================================================
# Case: a tty is available -> "nerv install" runs with stdin reattached,
# no --no-configure
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_TTY_OVERRIDE=1
export NERV_TTY_DEVICE=/dev/null
nerv_log="$(mktemp)"
export NERV_STUB_LOG="$nerv_log"
install_dir="$work_dir/install-tty"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
invocation="$(cat "$nerv_log" 2>/dev/null || true)"
if [ "$exit_code" -eq 0 ] && line_has_all "$invocation" 'install' && ! line_has_all "$invocation" '--no-configure'; then
  report "nerv-install-invoked-with-tty" 0
else
  report "nerv-install-invoked-with-tty" 1 "exit=${exit_code} invocation=[${invocation}]"
fi
rm -rf "$install_dir"
rm -f "$out" "$err" "$nerv_log"

# =============================================================================
# Case: no tty available -> "nerv install --no-configure" runs, hint printed
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_TTY_OVERRIDE=0
nerv_log="$(mktemp)"
export NERV_STUB_LOG="$nerv_log"
install_dir="$work_dir/install-no-tty"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
invocation="$(cat "$nerv_log" 2>/dev/null || true)"
if [ "$exit_code" -eq 0 ] \
  && line_has_all "$invocation" 'install' '--no-configure' \
  && grep -qi 'nerv configure' "$out"; then
  report "nerv-install-invoked-without-tty-no-configure" 0
else
  report "nerv-install-invoked-without-tty-no-configure" 1 "exit=${exit_code} invocation=[${invocation}]"
fi
rm -rf "$install_dir"
rm -f "$out" "$err" "$nerv_log"

# =============================================================================
# Case: "nerv install"'s exit code propagates as install.sh's own exit code
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_TTY_OVERRIDE=1
export NERV_TTY_DEVICE=/dev/null
export NERV_STUB_EXIT=7
nerv_log="$(mktemp)"
export NERV_STUB_LOG="$nerv_log"
install_dir="$work_dir/install-nonzero-exit"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 7 ]; then
  report "nerv-install-nonzero-exit-propagates" 0
else
  report "nerv-install-nonzero-exit-propagates" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err" "$nerv_log"

# =============================================================================
# Case: NERV_NO_INSTALL skips running "nerv install" entirely
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_NO_INSTALL=1
nerv_log="$(mktemp -u)"
export NERV_STUB_LOG="$nerv_log"
install_dir="$work_dir/install-no-install-flag"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 0 ] && [ -x "${install_dir}/nerv" ] && [ ! -e "$nerv_log" ]; then
  report "no-install-flag-skips-nerv-install" 0
else
  report "no-install-flag-skips-nerv-install" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err"

# =============================================================================
# Case: unsupported OS -> exit 1 with the "go install" hint
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="plan9"
export NERV_ARCH="$TEST_ARCH"
export NERV_NO_INSTALL=1
install_dir="$work_dir/install-bad-os"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -qi 'go install' "$err"; then
  report "unsupported-os-exit-1-with-hint" 0
else
  report "unsupported-os-exit-1-with-hint" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err"

# =============================================================================
# Case: unsupported architecture -> exit 1 with the "go install" hint
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="mips"
export NERV_NO_INSTALL=1
install_dir="$work_dir/install-bad-arch"
export NERV_INSTALL_DIR="$install_dir"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_normal"
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -qi 'go install' "$err"; then
  report "unsupported-arch-exit-1-with-hint" 0
else
  report "unsupported-arch-exit-1-with-hint" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err"

# =============================================================================
# Case: missing claude -> exit 1 with a hint (curl present)
# =============================================================================
reset_test_env
export NERV_API_BASE="$TEST_API_BASE"
export NERV_DOWNLOAD_BASE="$TEST_DOWNLOAD_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_OS="$TEST_OS"
export NERV_ARCH="$TEST_ARCH"
export NERV_NO_INSTALL=1
install_dir="$work_dir/install-no-claude"
export NERV_INSTALL_DIR="$install_dir"
stub_dir_curl_only="$work_dir/stub-curl-only"
mkdir -p "$stub_dir_curl_only"
write_curl_stub "$stub_dir_curl_only/curl"
path_no_claude_full="${stub_dir_curl_only}:${path_no_claude}"
out="$(mktemp)"
err="$(mktemp)"
run_install_sh "$out" "$err" "$path_no_claude_full"
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -q 'docs.claude.com' "$err"; then
  report "missing-claude-exit-1-with-hint" 0
else
  report "missing-claude-exit-1-with-hint" 1 "exit=${exit_code}"
fi
rm -rf "$install_dir"
rm -f "$out" "$err"

reset_test_env

echo ""
echo "Results: ${pass_count} passed, ${fail_count} failed, ${skip_count} skipped"

if [ "$fail_count" -ne 0 ]; then
  exit 1
fi
exit 0
