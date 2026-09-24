#!/usr/bin/env bash
# hook-session-start.test.sh
#
# Bash/POSIX-friendly assertions for plugin/hooks/nerv-session-start.sh
# (the SessionStart activation gate). No external deps beyond coreutils,
# grep and mktemp.
#
# Run with:
#   bash tests/hook-session-start.test.sh
#
# Exits 0 if every case passes, 1 if any case fails. Prints one
# "PASS <case>" / "FAIL <case>" line per case.

set -u

self_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${self_dir}/.." && pwd)"
hook_script="${repo_root}/plugin/hooks/nerv-session-start.sh"
plugin_root="${repo_root}/plugin"

pass_count=0
fail_count=0

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

# assert_empty CASE_NAME OUT EXIT
assert_empty() {
  local case_name="$1" out="$2" exit_code="$3"
  if [ -z "$out" ] && [ "$exit_code" -eq 0 ]; then
    report "$case_name" 0
  else
    report "$case_name" 1 "expected empty stdout + exit 0, got exit=${exit_code} stdout=[${out}]"
  fi
}

# assert_active CASE_NAME OUT EXIT
assert_active() {
  local case_name="$1" out="$2" exit_code="$3"
  local first_line
  first_line="$(printf '%s\n' "$out" | head -n 1)"
  if [ "$exit_code" -eq 0 ] \
    && [ "$first_line" = "# NERV orchestrator protocol (active: .nerv/nerv.yaml enabled)" ] \
    && printf '%s' "$out" | grep -q 'name: nerv-orchestrator'; then
    report "$case_name" 0
  else
    report "$case_name" 1 "expected active header + skill body + exit 0, got exit=${exit_code} first_line=[${first_line}]"
  fi
}

run_hook() {
  local project_dir="$1"
  CLAUDE_PLUGIN_ROOT="$plugin_root" CLAUDE_PROJECT_DIR="$project_dir" bash "$hook_script"
}

# --- Case 1: no .nerv/ dir at all -> empty stdout ---
tmp1="$(mktemp -d)"
out1="$(run_hook "$tmp1")"; exit1=$?
assert_empty "no-nerv-dir" "$out1" "$exit1"
rm -rf "$tmp1"

# --- Case 2: .nerv/nerv.yaml with enabled: false -> empty stdout ---
tmp2="$(mktemp -d)"
mkdir -p "$tmp2/.nerv"
printf 'enabled: false\n' > "$tmp2/.nerv/nerv.yaml"
out2="$(run_hook "$tmp2")"; exit2=$?
assert_empty "enabled-false" "$out2" "$exit2"
rm -rf "$tmp2"

# --- Case 3: enabled: true (LF) -> active ---
tmp3="$(mktemp -d)"
mkdir -p "$tmp3/.nerv"
printf 'enabled: true\n' > "$tmp3/.nerv/nerv.yaml"
out3="$(run_hook "$tmp3")"; exit3=$?
assert_active "enabled-true-lf" "$out3" "$exit3"
rm -rf "$tmp3"

# --- Case 4: enabled: true   # comment -> active ---
tmp4="$(mktemp -d)"
mkdir -p "$tmp4/.nerv"
printf 'enabled: true   # comment\n' > "$tmp4/.nerv/nerv.yaml"
out4="$(run_hook "$tmp4")"; exit4=$?
assert_active "enabled-true-inline-comment" "$out4" "$exit4"
rm -rf "$tmp4"

# --- Case 5: enabled: true\r\n (CRLF) -> active ---
tmp5="$(mktemp -d)"
mkdir -p "$tmp5/.nerv"
printf 'enabled: true\r\n' > "$tmp5/.nerv/nerv.yaml"
out5="$(run_hook "$tmp5")"; exit5=$?
assert_active "enabled-true-crlf" "$out5" "$exit5"
rm -rf "$tmp5"

# --- Case 6: enabled: truex -> empty stdout ---
tmp6="$(mktemp -d)"
mkdir -p "$tmp6/.nerv"
printf 'enabled: truex\n' > "$tmp6/.nerv/nerv.yaml"
out6="$(run_hook "$tmp6")"; exit6=$?
assert_empty "enabled-truex-rejected" "$out6" "$exit6"
rm -rf "$tmp6"

# --- Case 7: indented "  enabled: true" (e.g. nested under another key) ---
# Pinned behavior: the activation regex only strips CRLF and matches
# whitespace-surrounded "enabled: true" on its own line; it does not parse
# YAML nesting, so indentation alone does not disqualify the line and the
# hook activates. This case documents and pins that exact behavior rather
# than asserting empty stdout.
tmp7="$(mktemp -d)"
mkdir -p "$tmp7/.nerv"
printf 'other_key:\n  enabled: true\n' > "$tmp7/.nerv/nerv.yaml"
out7="$(run_hook "$tmp7")"; exit7=$?
assert_active "enabled-true-indented-pinned" "$out7" "$exit7"
rm -rf "$tmp7"

# --- Case 8: CLAUDE_PROJECT_DIR unset -> falls back to $PWD ---
tmp8="$(mktemp -d)"
mkdir -p "$tmp8/.nerv"
printf 'enabled: true\n' > "$tmp8/.nerv/nerv.yaml"
out8="$(cd "$tmp8" && CLAUDE_PLUGIN_ROOT="$plugin_root" env -u CLAUDE_PROJECT_DIR bash "$hook_script")"
exit8=$?
assert_active "pwd-fallback-no-project-dir" "$out8" "$exit8"
rm -rf "$tmp8"

echo ""
echo "Results: ${pass_count} passed, ${fail_count} failed"

if [ "$fail_count" -ne 0 ]; then
  exit 1
fi
exit 0
