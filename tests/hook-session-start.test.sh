#!/usr/bin/env bash
# hook-session-start.test.sh
#
# Bash/POSIX-friendly assertions for plugin/hooks/nerv-session-start.sh
# (the SessionStart activation gate). No external deps beyond coreutils,
# grep, chmod and mktemp.
#
# Run with:
#   bash tests/hook-session-start.test.sh
#
# Exits 0 if every case passes (or is skipped), 1 if any case fails.
# Prints one "PASS <case>" / "FAIL <case>" / "SKIP <case>" line per case.

set -u
set -o pipefail

self_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${self_dir}/.." && pwd)"
hook_script="${repo_root}/plugin/hooks/nerv-session-start.sh"
plugin_root="${repo_root}/plugin"

pass_count=0
fail_count=0
skip_count=0

report() {
  local case_name="$1"
  local ok="$2"
  local detail="${3:-}"
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

# run_capture OUT_FILE ERR_FILE CMD...
# Runs CMD with stdout -> OUT_FILE and stderr -> ERR_FILE. The function's own
# return value is CMD's exit code directly (captured from "$?" right after
# the command, not laundered through an intermediate command-substitution
# assignment), so callers read it straight off "$?".
run_capture() {
  local out_file="$1" err_file="$2"
  shift 2
  "$@" >"$out_file" 2>"$err_file"
}

# run_hook PROJECT_DIR OUT_FILE ERR_FILE
run_hook() {
  local project_dir="$1" out_file="$2" err_file="$3"
  run_capture "$out_file" "$err_file" \
    env CLAUDE_PLUGIN_ROOT="$plugin_root" CLAUDE_PROJECT_DIR="$project_dir" bash "$hook_script"
}

# assert_empty CASE_NAME OUT_FILE EXIT
assert_empty() {
  local case_name="$1" out_file="$2" exit_code="$3"
  local out
  out="$(cat "$out_file")"
  if [ -z "$out" ] && [ "$exit_code" -eq 0 ]; then
    report "$case_name" 0
  else
    report "$case_name" 1 "expected empty stdout + exit 0, got exit=${exit_code} stdout=[${out}]"
  fi
}

# assert_active CASE_NAME OUT_FILE EXIT
assert_active() {
  local case_name="$1" out_file="$2" exit_code="$3"
  local out first_line byte_count
  out="$(cat "$out_file")"
  first_line="$(head -n 1 "$out_file")"
  byte_count="$(wc -c <"$out_file" | tr -d ' ')"
  if [ "$exit_code" -eq 0 ] \
    && [ "$first_line" = "# NERV orchestrator protocol (active: .ai-workflow/config.yaml enabled)" ] \
    && [ "$byte_count" -lt 10000 ] \
    && grep -q 'nerv:nerv-orchestrator' "$out_file" \
    && ! grep -q 'name: nerv-orchestrator' "$out_file"; then
    report "$case_name" 0
  else
    report "$case_name" 1 "expected active header + Skill-tool instruction + no skill body + stdout <10000 bytes + exit 0, got exit=${exit_code} bytes=${byte_count} first_line=[${first_line}]"
  fi
}

# assert_empty_stdout_with_stderr CASE_NAME OUT_FILE ERR_FILE EXIT STDERR_PATTERN
assert_empty_stdout_with_stderr() {
  local case_name="$1" out_file="$2" err_file="$3" exit_code="$4" pattern="$5"
  local out err
  out="$(cat "$out_file")"
  err="$(cat "$err_file")"
  if [ -z "$out" ] && [ "$exit_code" -eq 0 ] && grep -q "$pattern" "$err_file"; then
    report "$case_name" 0
  else
    report "$case_name" 1 "expected empty stdout + exit 0 + stderr matching [${pattern}], got exit=${exit_code} stdout=[${out}] stderr=[${err}]"
  fi
}

# cfg DIR: creates DIR/.ai-workflow/ and prints the project config path.
cfg() {
  mkdir -p "$1/.ai-workflow"
  printf '%s' "$1/.ai-workflow/config.yaml"
}

# --- Case 1: no .ai-workflow/config.yaml at all -> empty stdout ---
tmp1="$(mktemp -d)"
out1="$(mktemp)"; err1="$(mktemp)"
run_hook "$tmp1" "$out1" "$err1"; exit1=$?
assert_empty "no-nerv-dir" "$out1" "$exit1"
rm -rf "$tmp1"; rm -f "$out1" "$err1"

# --- Case 2: .ai-workflow/config.yaml with enabled: false -> empty stdout ---
tmp2="$(mktemp -d)"
printf 'enabled: false\n' > "$(cfg "$tmp2")"
out2="$(mktemp)"; err2="$(mktemp)"
run_hook "$tmp2" "$out2" "$err2"; exit2=$?
assert_empty "enabled-false" "$out2" "$exit2"
rm -rf "$tmp2"; rm -f "$out2" "$err2"

# --- Case 3: enabled: true (LF) -> active ---
tmp3="$(mktemp -d)"
printf 'enabled: true\n' > "$(cfg "$tmp3")"
out3="$(mktemp)"; err3="$(mktemp)"
run_hook "$tmp3" "$out3" "$err3"; exit3=$?
assert_active "enabled-true-lf" "$out3" "$exit3"
rm -rf "$tmp3"; rm -f "$out3" "$err3"

# --- Case 4: enabled: true   # comment -> active ---
tmp4="$(mktemp -d)"
printf 'enabled: true   # comment\n' > "$(cfg "$tmp4")"
out4="$(mktemp)"; err4="$(mktemp)"
run_hook "$tmp4" "$out4" "$err4"; exit4=$?
assert_active "enabled-true-inline-comment" "$out4" "$exit4"
rm -rf "$tmp4"; rm -f "$out4" "$err4"

# --- Case 5: enabled: true\r\n (CRLF) -> active ---
tmp5="$(mktemp -d)"
printf 'enabled: true\r\n' > "$(cfg "$tmp5")"
out5="$(mktemp)"; err5="$(mktemp)"
run_hook "$tmp5" "$out5" "$err5"; exit5=$?
assert_active "enabled-true-crlf" "$out5" "$exit5"
rm -rf "$tmp5"; rm -f "$out5" "$err5"

# --- Case 6: enabled: truex -> empty stdout ---
tmp6="$(mktemp -d)"
printf 'enabled: truex\n' > "$(cfg "$tmp6")"
out6="$(mktemp)"; err6="$(mktemp)"
run_hook "$tmp6" "$out6" "$err6"; exit6=$?
assert_empty "enabled-truex-rejected" "$out6" "$exit6"
rm -rf "$tmp6"; rm -f "$out6" "$err6"

# --- Case 7: indented "  enabled: true" (nested under another key) ---
# Only the top-level enabled key activates NERV: a nested one (a plugin's
# own enabled flag, say) does not, so the hook prints nothing.
tmp7="$(mktemp -d)"
printf 'other_key:\n  enabled: true\n' > "$(cfg "$tmp7")"
out7="$(mktemp)"; err7="$(mktemp)"
run_hook "$tmp7" "$out7" "$err7"; exit7=$?
assert_empty "enabled-true-nested-ignored" "$out7" "$exit7"
rm -rf "$tmp7"; rm -f "$out7" "$err7"

# --- Case 7b: a top-level enabled: true after a nested enabled: false ---
tmp7b="$(mktemp -d)"
printf 'release:\n  enabled: false\nenabled: true\n' > "$(cfg "$tmp7b")"
out7b="$(mktemp)"; err7b="$(mktemp)"
run_hook "$tmp7b" "$out7b" "$err7b"; exit7b=$?
assert_active "enabled-true-top-level-after-nested" "$out7b" "$exit7b"
rm -rf "$tmp7b"; rm -f "$out7b" "$err7b"

# --- Case 8: CLAUDE_PROJECT_DIR unset -> falls back to $PWD ---
tmp8="$(mktemp -d)"
printf 'enabled: true\n' > "$(cfg "$tmp8")"
out8="$(mktemp)"; err8="$(mktemp)"
(cd "$tmp8" && run_capture "$out8" "$err8" env -u CLAUDE_PROJECT_DIR CLAUDE_PLUGIN_ROOT="$plugin_root" bash "$hook_script")
exit8=$?
assert_active "pwd-fallback-no-project-dir" "$out8" "$exit8"
rm -rf "$tmp8"; rm -f "$out8" "$err8"

# --- Case 9: CLAUDE_PLUGIN_ROOT unset -> defaults to the script's own
# plugin directory (derived from its own location) -> active ---
tmp9="$(mktemp -d)"
printf 'enabled: true\n' > "$(cfg "$tmp9")"
out9="$(mktemp)"; err9="$(mktemp)"
run_capture "$out9" "$err9" env -u CLAUDE_PLUGIN_ROOT CLAUDE_PROJECT_DIR="$tmp9" bash "$hook_script"
exit9=$?
assert_active "plugin-root-unset-defaults-to-script-location" "$out9" "$exit9"
rm -rf "$tmp9"; rm -f "$out9" "$err9"

# --- Case 10: CLAUDE_PLUGIN_ROOT points at an empty directory (skill file
# missing) -> NO header, no partial injection; one diagnostic on stderr ---
tmp10="$(mktemp -d)"
printf 'enabled: true\n' > "$(cfg "$tmp10")"
empty_plugin_root="$(mktemp -d)"
out10="$(mktemp)"; err10="$(mktemp)"
run_capture "$out10" "$err10" env CLAUDE_PLUGIN_ROOT="$empty_plugin_root" CLAUDE_PROJECT_DIR="$tmp10" bash "$hook_script"
exit10=$?
assert_empty_stdout_with_stderr "plugin-root-points-to-empty-dir" "$out10" "$err10" "$exit10" "protocol skill not found"
rm -rf "$tmp10" "$empty_plugin_root"; rm -f "$out10" "$err10"

# --- Case 11: .ai-workflow/config.yaml exists as a DIRECTORY, not a file -> the
# "-f" guard fails closed, empty stdout, exit 0 ---
tmp11="$(mktemp -d)"
mkdir -p "$tmp11/.ai-workflow/config.yaml"
out11="$(mktemp)"; err11="$(mktemp)"
run_hook "$tmp11" "$out11" "$err11"; exit11=$?
assert_empty "config-path-is-a-directory" "$out11" "$exit11"
rm -rf "$tmp11"; rm -f "$out11" "$err11"

# --- Case 12: config file present but unreadable (chmod 000) -> fail-safe
# empty stdout, exit 0. Skipped where chmod 000 does not actually block
# reads for the current user (e.g. root, or Windows/Git Bash). ---
tmp12="$(mktemp -d)"
printf 'enabled: true\n' > "$(cfg "$tmp12")"
chmod 000 "$tmp12/.ai-workflow/config.yaml" 2>/dev/null || true
if [ -r "$tmp12/.ai-workflow/config.yaml" ]; then
  skip "config-unreadable" "chmod 000 does not block reads on this platform"
else
  out12="$(mktemp)"; err12="$(mktemp)"
  run_hook "$tmp12" "$out12" "$err12"; exit12=$?
  assert_empty "config-unreadable" "$out12" "$exit12"
  rm -f "$out12" "$err12"
fi
chmod 600 "$tmp12/.ai-workflow/config.yaml" 2>/dev/null || true
rm -rf "$tmp12"

# --- Case 13: only the old .nerv/nerv.yaml, enabled: true -> NOT active;
# stdout is exactly the one-line "nerv migrate" notice ---
tmp13="$(mktemp -d)"
mkdir -p "$tmp13/.nerv"
printf 'enabled: true\n' > "$tmp13/.nerv/nerv.yaml"
out13="$(mktemp)"; err13="$(mktemp)"
run_hook "$tmp13" "$out13" "$err13"; exit13=$?
want13='NERV: .nerv/nerv.yaml is no longer read (configuration now lives in .ai-workflow/config.yaml); run "nerv migrate" to move it. NERV is not active in this session.'
if [ "$exit13" -eq 0 ] && [ "$(cat "$out13")" = "$want13" ]; then
  report "old-config-only-hints-migrate" 0
else
  report "old-config-only-hints-migrate" 1 "expected exactly [${want13}], got exit=${exit13} stdout=[$(cat "$out13")]"
fi
rm -rf "$tmp13"; rm -f "$out13" "$err13"

# --- Case 14: only the old file, enabled: false -> empty stdout ---
tmp14="$(mktemp -d)"
mkdir -p "$tmp14/.nerv"
printf 'enabled: false\n' > "$tmp14/.nerv/nerv.yaml"
out14="$(mktemp)"; err14="$(mktemp)"
run_hook "$tmp14" "$out14" "$err14"; exit14=$?
assert_empty "old-config-disabled-silent" "$out14" "$exit14"
rm -rf "$tmp14"; rm -f "$out14" "$err14"

# --- Case 15: both files; the new one decides (enabled: false) and the
# old one is ignored -> empty stdout, no notice ---
tmp15="$(mktemp -d)"
mkdir -p "$tmp15/.nerv"
printf 'enabled: true\n' > "$tmp15/.nerv/nerv.yaml"
printf 'enabled: false\n' > "$(cfg "$tmp15")"
out15="$(mktemp)"; err15="$(mktemp)"
run_hook "$tmp15" "$out15" "$err15"; exit15=$?
assert_empty "new-config-wins-over-old" "$out15" "$exit15"
rm -rf "$tmp15"; rm -f "$out15" "$err15"

# --- Case 16 (S9): only the previous .ai-workflow.yml, enabled: true ->
# NOT active; stdout is exactly the one-line "nerv migrate" notice ---
want_prev='NERV: .ai-workflow.yml is no longer read (configuration now lives in .ai-workflow/config.yaml); run "nerv migrate" to move it. NERV is not active in this session.'
tmp16="$(mktemp -d)"
printf 'enabled: true\n' > "$tmp16/.ai-workflow.yml"
out16="$(mktemp)"; err16="$(mktemp)"
run_hook "$tmp16" "$out16" "$err16"; exit16=$?
if [ "$exit16" -eq 0 ] && [ "$(cat "$out16")" = "$want_prev" ]; then
  report "previous-config-hints-migrate" 0
else
  report "previous-config-hints-migrate" 1 "expected exactly [${want_prev}], got exit=${exit16} stdout=[$(cat "$out16")]"
fi
rm -rf "$tmp16"; rm -f "$out16" "$err16"

# --- Case 17 (S9): a leftover .ai-workflow.yml next to an enabling
# .ai-workflow/config.yaml -> NOT active (never read silently); the notice ---
tmp17="$(mktemp -d)"
printf 'enabled: false\n' > "$tmp17/.ai-workflow.yml"
printf 'enabled: true\n' > "$(cfg "$tmp17")"
out17="$(mktemp)"; err17="$(mktemp)"
run_hook "$tmp17" "$out17" "$err17"; exit17=$?
if [ "$exit17" -eq 0 ] && [ "$(cat "$out17")" = "$want_prev" ]; then
  report "leftover-previous-config-blocks-activation" 0
else
  report "leftover-previous-config-blocks-activation" 1 "expected exactly [${want_prev}], got exit=${exit17} stdout=[$(cat "$out17")]"
fi
rm -rf "$tmp17"; rm -f "$out17" "$err17"

# --- Case 18 (S9): only a previous .ai-workflow.yml, enabled: false ->
# empty stdout (NERV stays invisible in a repository that never opted in) ---
tmp18="$(mktemp -d)"
printf 'enabled: false\n' > "$tmp18/.ai-workflow.yml"
out18="$(mktemp)"; err18="$(mktemp)"
run_hook "$tmp18" "$out18" "$err18"; exit18=$?
assert_empty "previous-config-disabled-silent" "$out18" "$exit18"
rm -rf "$tmp18"; rm -f "$out18" "$err18"

echo ""
echo "Results: ${pass_count} passed, ${fail_count} failed, ${skip_count} skipped"

if [ "$fail_count" -ne 0 ]; then
  exit 1
fi
exit 0
