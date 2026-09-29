#!/usr/bin/env bash
# get-nerv.test.sh
#
# Bash/POSIX-friendly assertions for get-nerv.sh (the curl|bash / wget|bash
# bootstrap installer). No external deps beyond git, coreutils, and
# optionally python3 (present here; the sed/grep JSON fallback is exercised
# explicitly via NERV_JSON_PARSER=sed regardless of python3 availability).
#
# Run with:
#   bash tests/get-nerv.test.sh
#
# Exits 0 if every case passes, 1 if any case fails.
# Prints one "PASS <case>" / "FAIL <case>" / "SKIP <case>" line per case.
#
# Test seams introduced by get-nerv.sh that this suite relies on:
#   NERV_REPO_URL       git remote to clone/fetch (points at a local bare repo)
#   NERV_API_BASE       GitHub API base (matched by suffix in the curl stub)
#   NERV_JSON_PARSER    force "python3" or "sed" (bypasses autodetection)
#   NERV_TTY_OVERRIDE   "1"/"0" forces has_tty() instead of probing /dev/tty
#   NERV_TTY_DEVICE     device to read the wizard's input from when a tty is
#                       available (tests use /dev/null; never a real tty)
# Plus stub-only environment variables read by the stub executables below:
#   NERV_STUB_PWSH_LOG, NERV_STUB_PWSH_MAJOR, NERV_STUB_PWSH_EXIT,
#   NERV_STUB_CLAUDE_EXIT, NERV_STUB_CURL_LOG, NERV_FIXTURES

set -u
set -o pipefail

self_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${self_dir}/.." && pwd)"
get_nerv_script="${repo_root}/get-nerv.sh"

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
# fully unresolvable, while every unrelated directory (and thus
# grep/sed/mktemp/etc.) stays available. Loops because a machine can expose
# the same tool from more than one PATH entry (e.g. two "git" locations).
# No-op when BIN_NAME is not found at all.
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

# run_get_nerv OUT_FILE ERR_FILE PATH_VALUE -- ARGS... — runs get-nerv.sh
# with PATH replaced by PATH_VALUE and every currently-exported NERV_*/stub
# variable inherited as-is. Returns get-nerv.sh's own exit code.
run_get_nerv() {
  local out_file="$1" err_file="$2" path_val="$3"
  shift 3
  if [ "${1:-}" = "--" ]; then shift; fi
  PATH="$path_val" bash "$get_nerv_script" "$@" >"$out_file" 2>"$err_file"
}

reset_test_env() {
  unset NERV_REPO_URL NERV_API_BASE NERV_CHANNEL NERV_HOME NERV_NO_CONFIGURE \
    NERV_JSON_PARSER NERV_TTY_OVERRIDE NERV_TTY_DEVICE NERV_FIXTURES \
    NERV_STUB_PWSH_LOG NERV_STUB_PWSH_MAJOR NERV_STUB_PWSH_EXIT \
    NERV_STUB_CLAUDE_EXIT NERV_STUB_CURL_LOG 2>/dev/null || true
}

write_pwsh_stub() {
  cat >"$1" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
: "${NERV_STUB_PWSH_LOG:?NERV_STUB_PWSH_LOG not set}"
printf '%s\n' "$*" >>"$NERV_STUB_PWSH_LOG"
for arg in "$@"; do
  if [ "$arg" = "-Command" ]; then
    printf '%s\n' "${NERV_STUB_PWSH_MAJOR:-7}"
    exit 0
  fi
done
exit "${NERV_STUB_PWSH_EXIT:-0}"
STUB
  chmod +x "$1"
}

write_claude_stub() {
  cat >"$1" <<'STUB'
#!/usr/bin/env bash
exit "${NERV_STUB_CLAUDE_EXIT:-0}"
STUB
  chmod +x "$1"
}

write_curl_stub() {
  cat >"$1" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
: "${NERV_FIXTURES:?NERV_FIXTURES not set}"
: "${NERV_STUB_CURL_LOG:?NERV_STUB_CURL_LOG not set}"
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
case "$url" in
  */releases/latest) cp "${NERV_FIXTURES}/latest.json" "$outfile" ;;
  *releases*per_page*) cp "${NERV_FIXTURES}/releases.json" "$outfile" ;;
  *) : >"$outfile" ;;
esac
exit 0
STUB
  chmod +x "$1"
}

init_repo() {
  local dir="$1"
  mkdir -p "$dir"
  git -C "$dir" init -q
  git -C "$dir" symbolic-ref HEAD refs/heads/main
  git -C "$dir" config user.email "nerv-test@example.com"
  git -C "$dir" config user.name "NERV Test"
}

commit_marker() {
  local dir="$1" text="$2" message="$3"
  mkdir -p "$dir/plugin/tools"
  printf '%s\n' "$text" >>"$dir/plugin/tools/install.ps1"
  git -C "$dir" add -A
  git -C "$dir" commit -q -m "$message"
}

# build_source_repo DIR — a non-bare working repo with main/develop/
# release-0.2.0 branches and tags v0.1.0, v0.2.0-alpha.1, v0.2.0-rc.1,
# each containing plugin/tools/install.ps1 (so the delegation path exists).
build_source_repo() {
  local dir="$1"
  init_repo "$dir"
  commit_marker "$dir" "dummy install.ps1 fixture" "chore: v0.1.0 fixture"
  git -C "$dir" tag v0.1.0

  git -C "$dir" checkout -q -b develop
  commit_marker "$dir" "develop marker" "chore: develop commit"
  git -C "$dir" tag v0.2.0-alpha.1

  git -C "$dir" checkout -q -b release/0.2.0
  commit_marker "$dir" "rc marker" "chore: rc commit"
  git -C "$dir" tag v0.2.0-rc.1

  git -C "$dir" checkout -q main
}

make_bare() {
  git clone --bare -q "$1" "$2"
}

TEST_API_BASE="https://fixture.test/repos/war-apps/nerv-gentle-ai"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

# --- Shared fixtures: one pristine source + bare repo, reused read-only ---
src_repo="$work_dir/src"
build_source_repo "$src_repo"
bare_repo="$work_dir/bare.git"
make_bare "$src_repo" "$bare_repo"

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

# --- Stub executables ---
stub_dir_full="$work_dir/stub-full"
mkdir -p "$stub_dir_full"
write_pwsh_stub "$stub_dir_full/pwsh"
write_claude_stub "$stub_dir_full/claude"
write_curl_stub "$stub_dir_full/curl"

# --- PATH variants derived from this test's own (real) PATH ---
original_path="$PATH"
path_no_git="$(strip_dir_of "$original_path" git)"
path_no_pwsh="$(strip_dir_of "$original_path" pwsh)"
path_no_claude="$(strip_dir_of "$original_path" claude)"
path_base_no_tools="$(strip_dir_of "$(strip_dir_of "$(strip_dir_of "$original_path" pwsh)" claude)" curl)"
path_normal="${stub_dir_full}:${path_base_no_tools}"

# =============================================================================
# Case: --help
# =============================================================================
reset_test_env
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$original_path" -- --help
exit_code=$?
if [ "$exit_code" -eq 0 ] && grep -q '^Usage: get-nerv.sh' "$out"; then
  report "help-exit-0" 0
else
  report "help-exit-0" 1 "exit=${exit_code}"
fi
rm -f "$out" "$err"

# =============================================================================
# Case: unknown flag -> usage on stderr, exit 2
# =============================================================================
reset_test_env
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$original_path" -- --bogus-flag
exit_code=$?
if [ "$exit_code" -eq 2 ] && grep -q 'Usage: get-nerv.sh' "$err"; then
  report "unknown-flag-exit-2" 0
else
  report "unknown-flag-exit-2" 1 "exit=${exit_code}"
fi
rm -f "$out" "$err"

# =============================================================================
# Case: unknown channel -> usage on stderr, exit 2
# =============================================================================
reset_test_env
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$original_path" -- --channel bogus
exit_code=$?
if [ "$exit_code" -eq 2 ] && grep -q 'Usage: get-nerv.sh' "$err"; then
  report "unknown-channel-exit-2" 0
else
  report "unknown-channel-exit-2" 1 "exit=${exit_code}"
fi
rm -f "$out" "$err"

# =============================================================================
# Case: missing git -> exit 1 with a platform hint
# =============================================================================
reset_test_env
target_dir="$work_dir/target-missing-git"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_no_git" -- --dir "$target_dir"
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -q 'git-scm.com' "$err"; then
  report "missing-git-exit-1-with-hint" 0
else
  report "missing-git-exit-1-with-hint" 1 "exit=${exit_code}"
fi
rm -f "$out" "$err"

# =============================================================================
# Case: missing pwsh -> exit 1 with a platform hint (real git present)
# =============================================================================
reset_test_env
target_dir="$work_dir/target-missing-pwsh"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_no_pwsh" -- --dir "$target_dir"
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -Eq 'aka\.ms/install-powershell|brew install --cask powershell|learn\.microsoft\.com' "$err"; then
  report "missing-pwsh-exit-1-with-hint" 0
else
  report "missing-pwsh-exit-1-with-hint" 1 "exit=${exit_code}"
fi
rm -f "$out" "$err"

# =============================================================================
# Case: missing claude -> exit 1 with a hint (real git + real pwsh present)
# =============================================================================
reset_test_env
target_dir="$work_dir/target-missing-claude"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_no_claude" -- --dir "$target_dir"
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -q 'docs.claude.com' "$err"; then
  report "missing-claude-exit-1-with-hint" 0
else
  report "missing-claude-exit-1-with-hint" 1 "exit=${exit_code}"
fi
rm -f "$out" "$err"

# =============================================================================
# Case: pwsh major version 6 -> exit 1 (real git present, pwsh stubbed)
# =============================================================================
reset_test_env
stub_dir_pwsh6="$work_dir/stub-pwsh6"
mkdir -p "$stub_dir_pwsh6"
write_pwsh_stub "$stub_dir_pwsh6/pwsh"
target_dir="$work_dir/target-pwsh6"
out="$(mktemp)"; err="$(mktemp)"
pwsh_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_PWSH_MAJOR="6"
run_get_nerv "$out" "$err" "${stub_dir_pwsh6}:${path_no_pwsh}" -- --dir "$target_dir"
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -q '7 or newer' "$err"; then
  report "pwsh-major-6-exit-1" 0
else
  report "pwsh-major-6-exit-1" 1 "exit=${exit_code}"
fi
rm -f "$out" "$err" "$pwsh_log"

# =============================================================================
# Case: stable resolves v0.1.0 and clones with --branch v0.1.0
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_common"
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-stable"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel stable --dir "$target_dir" --no-configure
exit_code=$?
resolved_tag="$(git -C "$target_dir" describe --tags --exact-match 2>/dev/null)" || resolved_tag=""
if [ "$exit_code" -eq 0 ] && [ "$resolved_tag" = "v0.1.0" ]; then
  report "stable-resolves-v0.1.0" 0
else
  report "stable-resolves-v0.1.0" 1 "exit=${exit_code} tag=[${resolved_tag}]"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: alpha resolves v0.2.0-alpha.1
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_common"
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-alpha"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel alpha --dir "$target_dir" --no-configure
exit_code=$?
resolved_tag="$(git -C "$target_dir" describe --tags --exact-match 2>/dev/null)" || resolved_tag=""
if [ "$exit_code" -eq 0 ] && [ "$resolved_tag" = "v0.2.0-alpha.1" ]; then
  report "alpha-resolves-v0.2.0-alpha.1" 0
else
  report "alpha-resolves-v0.2.0-alpha.1" 1 "exit=${exit_code} tag=[${resolved_tag}]"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: rc resolves v0.2.0-rc.1
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_common"
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-rc"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel rc --dir "$target_dir" --no-configure
exit_code=$?
resolved_tag="$(git -C "$target_dir" describe --tags --exact-match 2>/dev/null)" || resolved_tag=""
if [ "$exit_code" -eq 0 ] && [ "$resolved_tag" = "v0.2.0-rc.1" ]; then
  report "rc-resolves-v0.2.0-rc.1" 0
else
  report "rc-resolves-v0.2.0-rc.1" 1 "exit=${exit_code} tag=[${resolved_tag}]"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: alpha with an empty releases list falls back to the develop branch
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_empty"
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-alpha-fallback"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel alpha --dir "$target_dir" --no-configure
exit_code=$?
head_branch="$(git -C "$target_dir" rev-parse --abbrev-ref HEAD 2>/dev/null)" || head_branch=""
if [ "$exit_code" -eq 0 ] && [ "$head_branch" = "develop" ]; then
  report "alpha-empty-releases-falls-back-to-develop" 0
else
  report "alpha-empty-releases-falls-back-to-develop" 1 "exit=${exit_code} head=[${head_branch}]"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: rc with no pre-release published yet -> exit 1
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_empty"
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-rc-none"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel rc --dir "$target_dir" --no-configure
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -qi 'no rc pre-release' "$err"; then
  report "rc-empty-releases-exit-1" 0
else
  report "rc-empty-releases-exit-1" 1 "exit=${exit_code}"
fi
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: existing non-git directory -> refuse, exit 1
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_common"
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-nongit"
mkdir -p "$target_dir"
printf 'not a repo\n' >"$target_dir/marker.txt"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel stable --dir "$target_dir" --no-configure
exit_code=$?
if [ "$exit_code" -eq 1 ] && grep -qi 'not a git repository' "$err"; then
  report "non-git-existing-dir-exit-1" 0
else
  report "non-git-existing-dir-exit-1" 1 "exit=${exit_code}"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: second run updates the existing checkout instead of re-cloning
# =============================================================================
reset_test_env
src_update="$work_dir/src-update"
build_source_repo "$src_update"
bare_update="$work_dir/bare-update.git"
make_bare "$src_update" "$bare_update"

fixtures_update="$work_dir/fixtures-update"
mkdir -p "$fixtures_update"
cat >"$fixtures_update/latest.json" <<'JSON'
{
  "tag_name": "v0.1.0",
  "prerelease": false,
  "name": "v0.1.0"
}
JSON

export NERV_REPO_URL="$bare_update"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_update"
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-update"
out1="$(mktemp)"; err1="$(mktemp)"
run_get_nerv "$out1" "$err1" "$path_normal" -- --channel stable --dir "$target_dir" --no-configure
first_exit=$?

marker_file="$target_dir/.nerv-test-marker"
printf 'sentinel\n' >"$marker_file"

git -C "$src_update" checkout -q main
printf 'update marker\n' >>"$src_update/plugin/tools/install.ps1"
git -C "$src_update" commit -q -am "chore: v0.1.1 fixture"
git -C "$src_update" tag v0.1.1
git -C "$src_update" push -q "$bare_update" main v0.1.1

cat >"$fixtures_update/latest.json" <<'JSON'
{
  "tag_name": "v0.1.1",
  "prerelease": false,
  "name": "v0.1.1"
}
JSON
: >"$curl_log"

out2="$(mktemp)"; err2="$(mktemp)"
run_get_nerv "$out2" "$err2" "$path_normal" -- --channel stable --dir "$target_dir" --no-configure
second_exit=$?
resolved_tag2="$(git -C "$target_dir" describe --tags --exact-match 2>/dev/null)" || resolved_tag2=""
marker_survived=1
[ -f "$marker_file" ] && marker_survived=0

if [ "$first_exit" -eq 0 ] && [ "$second_exit" -eq 0 ] && [ "$marker_survived" -eq 0 ] && [ "$resolved_tag2" = "v0.1.1" ]; then
  report "second-run-updates-not-clones" 0
else
  report "second-run-updates-not-clones" 1 "first=${first_exit} second=${second_exit} marker_survived=${marker_survived} tag=[${resolved_tag2}]"
fi
rm -rf "$target_dir"
rm -f "$out1" "$err1" "$out2" "$err2" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: pwsh invocation carries -RefreshCache -Skills -Configure when a tty
# is available (simulated via NERV_TTY_OVERRIDE=1, see has_tty() seam)
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_TTY_OVERRIDE=1
export NERV_TTY_DEVICE=/dev/null
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-tty"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel stable --dir "$target_dir"
exit_code=$?
file_line="$(grep -- '-File' "$pwsh_log")" || file_line=""
if [ "$exit_code" -eq 0 ] && line_has_all "$file_line" '-File' '-RefreshCache' '-Skills' '-Configure'; then
  report "pwsh-invocation-with-configure-tty" 0
else
  report "pwsh-invocation-with-configure-tty" 1 "exit=${exit_code} line=[${file_line}]"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: --no-configure omits -Configure regardless of tty availability
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_TTY_OVERRIDE=1
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-no-configure-flag"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel stable --dir "$target_dir" --no-configure
exit_code=$?
file_line="$(grep -- '-File' "$pwsh_log")" || file_line=""
if [ "$exit_code" -eq 0 ] && line_has_all "$file_line" '-File' '-RefreshCache' '-Skills' && ! line_has_all "$file_line" '-Configure'; then
  report "pwsh-invocation-without-configure-flag" 0
else
  report "pwsh-invocation-without-configure-flag" 1 "exit=${exit_code} line=[${file_line}]"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: no tty available -> -Configure omitted, skip message printed
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_TTY_OVERRIDE=0
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-no-tty"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel stable --dir "$target_dir"
exit_code=$?
file_line="$(grep -- '-File' "$pwsh_log")" || file_line=""
if [ "$exit_code" -eq 0 ] \
  && line_has_all "$file_line" '-File' '-RefreshCache' '-Skills' \
  && ! line_has_all "$file_line" '-Configure' \
  && grep -qi 'skipping the configuration wizard' "$out" \
  && grep -q 'configure.ps1' "$out"; then
  report "pwsh-invocation-without-configure-no-tty" 0
else
  report "pwsh-invocation-without-configure-no-tty" 1 "exit=${exit_code} line=[${file_line}]"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: pwsh's non-zero exit code propagates as get-nerv.sh's exit code
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_STUB_PWSH_EXIT=5
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-pwsh-fail"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel stable --dir "$target_dir" --no-configure
exit_code=$?
if [ "$exit_code" -eq 5 ]; then
  report "pwsh-nonzero-exit-propagates" 0
else
  report "pwsh-nonzero-exit-propagates" 1 "exit=${exit_code}"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: stable resolution via the sed/grep JSON fallback (NERV_JSON_PARSER=sed)
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_JSON_PARSER=sed
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-stable-sed"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel stable --dir "$target_dir" --no-configure
exit_code=$?
resolved_tag="$(git -C "$target_dir" describe --tags --exact-match 2>/dev/null)" || resolved_tag=""
if [ "$exit_code" -eq 0 ] && [ "$resolved_tag" = "v0.1.0" ]; then
  report "stable-resolves-v0.1.0-sed-fallback" 0
else
  report "stable-resolves-v0.1.0-sed-fallback" 1 "exit=${exit_code} tag=[${resolved_tag}]"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

# =============================================================================
# Case: alpha resolution via the sed/grep JSON fallback (NERV_JSON_PARSER=sed)
# =============================================================================
reset_test_env
export NERV_REPO_URL="$bare_repo"
export NERV_API_BASE="$TEST_API_BASE"
export NERV_FIXTURES="$fixtures_common"
export NERV_JSON_PARSER=sed
pwsh_log="$(mktemp)"; curl_log="$(mktemp)"
export NERV_STUB_PWSH_LOG="$pwsh_log"
export NERV_STUB_CURL_LOG="$curl_log"
target_dir="$work_dir/target-alpha-sed"
out="$(mktemp)"; err="$(mktemp)"
run_get_nerv "$out" "$err" "$path_normal" -- --channel alpha --dir "$target_dir" --no-configure
exit_code=$?
resolved_tag="$(git -C "$target_dir" describe --tags --exact-match 2>/dev/null)" || resolved_tag=""
if [ "$exit_code" -eq 0 ] && [ "$resolved_tag" = "v0.2.0-alpha.1" ]; then
  report "alpha-resolves-v0.2.0-alpha.1-sed-fallback" 0
else
  report "alpha-resolves-v0.2.0-alpha.1-sed-fallback" 1 "exit=${exit_code} tag=[${resolved_tag}]"
fi
rm -rf "$target_dir"
rm -f "$out" "$err" "$pwsh_log" "$curl_log"

reset_test_env

echo ""
echo "Results: ${pass_count} passed, ${fail_count} failed, ${skip_count} skipped"

if [ "$fail_count" -ne 0 ]; then
  exit 1
fi
exit 0
