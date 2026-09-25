#!/usr/bin/env bash
# hook-engram-project.test.sh
#
# Bash/POSIX-friendly assertions for plugin/hooks/nerv-engram-project.sh
# (the Engram project-detection SessionStart hook). No external deps beyond
# coreutils, grep, sed, git, chmod and mktemp.
#
# Run with:
#   bash tests/hook-engram-project.test.sh
#
# Exits 0 if every case passes (or is skipped), 1 if any case fails.
# Prints one "PASS <case>" / "FAIL <case>" / "SKIP <case>" line per case.

set -u
set -o pipefail

self_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${self_dir}/.." && pwd)"
hook_script="${repo_root}/plugin/hooks/nerv-engram-project.sh"
plugin_root="${repo_root}/plugin"

# Exact wording the hook must print, pinned here so a wording drift fails
# a test instead of silently changing user-facing behavior.
kb_line='NERV knowledge base: Engram project "nerv" — read precedents there before deciding, mirror decisions there (see nerv-phase-common.md).'
fallback_line='Engram project: nerv (source: NERV fallback — the repo resolved no project; pass project: "nerv" on every Engram write)'
ask_line='Engram project: undetermined — before the first Engram write (mem_save, mem_session_summary, mem_context) ask the user ONE question: general knowledge for the "root" project, or which named project? Then pass project: "<answer>" (with the recovery token when the error returned one). Never pick a project silently.'

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

# assert_output_exact CASE_NAME OUT_FILE EXIT EXPECTED
# EXPECTED is the full expected stdout, one printf-style arg per line
# (joined with newlines); compared byte-for-byte against actual stdout.
assert_output_exact() {
  local case_name="$1" out_file="$2" exit_code="$3"
  shift 3
  local expected
  expected="$(printf '%s\n' "$@")"
  local out
  out="$(cat "$out_file")"
  if [ "$exit_code" -eq 0 ] && [ "$out" = "$expected" ]; then
    report "$case_name" 0
  else
    report "$case_name" 1 "expected exit 0 + stdout=[${expected}], got exit=${exit_code} stdout=[${out}]"
  fi
}

# assert_no_git_repo checks the harness precondition a case relies on: a
# freshly mktemp'd directory must not resolve to an enclosing git repo, or
# the "undetermined" cases below would spuriously detect one.
assert_no_git_repo() {
  local dir="$1"
  ! git -C "$dir" rev-parse --show-toplevel >/dev/null 2>&1
}

git_available() {
  command -v git >/dev/null 2>&1
}

# --- Case 1: .engram/config.json wins over git toplevel detection ---
if git_available; then
  tmp1="$(mktemp -d)"
  (cd "$tmp1" && git init -q .)
  mkdir -p "$tmp1/.engram"
  printf '{"project_name": "configwins"}\n' > "$tmp1/.engram/config.json"
  out1="$(mktemp)"; err1="$(mktemp)"
  run_hook "$tmp1" "$out1" "$err1"; exit1=$?
  assert_output_exact "config-wins-over-git" "$out1" "$exit1" \
    'Engram project: configwins (source: .engram/config.json)'
  rm -rf "$tmp1"; rm -f "$out1" "$err1"
else
  skip "config-wins-over-git" "git not available"
fi

# --- Case 2: git toplevel basename, no config.json, not NERV ---
if git_available; then
  tmp2="$(mktemp -d)"
  (cd "$tmp2" && git init -q .)
  base2="$(basename "$tmp2")"
  out2="$(mktemp)"; err2="$(mktemp)"
  run_hook "$tmp2" "$out2" "$err2"; exit2=$?
  assert_output_exact "git-toplevel-basename" "$out2" "$exit2" \
    "Engram project: ${base2} (source: git toplevel)"
  rm -rf "$tmp2"; rm -f "$out2" "$err2"
else
  skip "git-toplevel-basename" "git not available"
fi

# --- Case 3: nested subfolder of a repo still reports the toplevel name ---
if git_available; then
  tmp3="$(mktemp -d)"
  (cd "$tmp3" && git init -q .)
  base3="$(basename "$tmp3")"
  mkdir -p "$tmp3/nested/deeper"
  out3="$(mktemp)"; err3="$(mktemp)"
  run_hook "$tmp3/nested/deeper" "$out3" "$err3"; exit3=$?
  assert_output_exact "nested-subfolder-reports-toplevel" "$out3" "$exit3" \
    "Engram project: ${base3} (source: git toplevel)"
  rm -rf "$tmp3"; rm -f "$out3" "$err3"
else
  skip "nested-subfolder-reports-toplevel" "git not available"
fi

# --- Case 4: no git + no config + not NERV -> undetermined ask line only,
# no knowledge-base line ---
tmp4="$(mktemp -d)"
if assert_no_git_repo "$tmp4"; then
  out4="$(mktemp)"; err4="$(mktemp)"
  run_hook "$tmp4" "$out4" "$err4"; exit4=$?
  assert_output_exact "no-git-no-config-not-nerv-asks" "$out4" "$exit4" "$ask_line"
  rm -f "$out4" "$err4"
else
  skip "no-git-no-config-not-nerv-asks" "temp dir unexpectedly resolves inside a git repo"
fi
rm -rf "$tmp4"

# --- Case 5: NERV enabled + git detected -> git line + knowledge-base
# line, no fallback line ---
if git_available; then
  tmp5="$(mktemp -d)"
  (cd "$tmp5" && git init -q .)
  base5="$(basename "$tmp5")"
  mkdir -p "$tmp5/.nerv"
  printf 'enabled: true\n' > "$tmp5/.nerv/nerv.yaml"
  out5="$(mktemp)"; err5="$(mktemp)"
  run_hook "$tmp5" "$out5" "$err5"; exit5=$?
  assert_output_exact "nerv-enabled-git-detected" "$out5" "$exit5" \
    "Engram project: ${base5} (source: git toplevel)" \
    "$kb_line"
  rm -rf "$tmp5"; rm -f "$out5" "$err5"
else
  skip "nerv-enabled-git-detected" "git not available"
fi

# --- Case 6: NERV enabled + undetermined -> knowledge-base line + nerv
# fallback line, no ask line ---
tmp6="$(mktemp -d)"
if assert_no_git_repo "$tmp6"; then
  mkdir -p "$tmp6/.nerv"
  printf 'enabled: true\n' > "$tmp6/.nerv/nerv.yaml"
  out6="$(mktemp)"; err6="$(mktemp)"
  run_hook "$tmp6" "$out6" "$err6"; exit6=$?
  assert_output_exact "nerv-enabled-undetermined-fallback" "$out6" "$exit6" \
    "$kb_line" \
    "$fallback_line"
  rm -f "$out6" "$err6"
else
  skip "nerv-enabled-undetermined-fallback" "temp dir unexpectedly resolves inside a git repo"
fi
rm -rf "$tmp6"

# --- Case 7: .engram/config.json with CRLF line endings and spaces around
# the colon still extracts project_name ---
tmp7="$(mktemp -d)"
if assert_no_git_repo "$tmp7"; then
  mkdir -p "$tmp7/.engram"
  printf '{\r\n  "project_name" : "crlf-proj"\r\n}\r\n' > "$tmp7/.engram/config.json"
  out7="$(mktemp)"; err7="$(mktemp)"
  run_hook "$tmp7" "$out7" "$err7"; exit7=$?
  assert_output_exact "config-crlf-and-spaces" "$out7" "$exit7" \
    'Engram project: crlf-proj (source: .engram/config.json)'
  rm -f "$out7" "$err7"
else
  skip "config-crlf-and-spaces" "temp dir unexpectedly resolves inside a git repo"
fi
rm -rf "$tmp7"

# --- Case 8: "enabled: true # comment" still counts as enabled ---
tmp8="$(mktemp -d)"
if assert_no_git_repo "$tmp8"; then
  mkdir -p "$tmp8/.nerv"
  printf 'enabled: true   # comment\n' > "$tmp8/.nerv/nerv.yaml"
  out8="$(mktemp)"; err8="$(mktemp)"
  run_hook "$tmp8" "$out8" "$err8"; exit8=$?
  assert_output_exact "nerv-enabled-inline-comment" "$out8" "$exit8" \
    "$kb_line" \
    "$fallback_line"
  rm -f "$out8" "$err8"
else
  skip "nerv-enabled-inline-comment" "temp dir unexpectedly resolves inside a git repo"
fi
rm -rf "$tmp8"

# --- Case 9: hook always exits 0 and prints nothing to stderr on a
# representative set of happy paths (config-only and git-only) ---
tmp9a="$(mktemp -d)"
mkdir -p "$tmp9a/.engram"
printf '{"project_name": "quiet-stderr"}\n' > "$tmp9a/.engram/config.json"
out9a="$(mktemp)"; err9a="$(mktemp)"
run_hook "$tmp9a" "$out9a" "$err9a"; exit9a=$?
err9a_content="$(cat "$err9a")"
if [ "$exit9a" -eq 0 ] && [ -z "$err9a_content" ]; then
  report "no-stderr-on-config-happy-path" 0
else
  report "no-stderr-on-config-happy-path" 1 "exit=${exit9a} stderr=[${err9a_content}]"
fi
rm -rf "$tmp9a"; rm -f "$out9a" "$err9a"

if git_available; then
  tmp9b="$(mktemp -d)"
  (cd "$tmp9b" && git init -q .)
  out9b="$(mktemp)"; err9b="$(mktemp)"
  run_hook "$tmp9b" "$out9b" "$err9b"; exit9b=$?
  err9b_content="$(cat "$err9b")"
  if [ "$exit9b" -eq 0 ] && [ -z "$err9b_content" ]; then
    report "no-stderr-on-git-happy-path" 0
  else
    report "no-stderr-on-git-happy-path" 1 "exit=${exit9b} stderr=[${err9b_content}]"
  fi
  rm -rf "$tmp9b"; rm -f "$out9b" "$err9b"
else
  skip "no-stderr-on-git-happy-path" "git not available"
fi

# --- Case 10: CLAUDE_PROJECT_DIR unset -> falls back to $PWD without
# crashing, and still detects via git toplevel from that PWD ---
if git_available; then
  tmp10="$(mktemp -d)"
  (cd "$tmp10" && git init -q .)
  base10="$(basename "$tmp10")"
  out10="$(mktemp)"; err10="$(mktemp)"
  (cd "$tmp10" && run_capture "$out10" "$err10" env -u CLAUDE_PROJECT_DIR CLAUDE_PLUGIN_ROOT="$plugin_root" bash "$hook_script")
  exit10=$?
  assert_output_exact "pwd-fallback-no-project-dir" "$out10" "$exit10" \
    "Engram project: ${base10} (source: git toplevel)"
  rm -rf "$tmp10"; rm -f "$out10" "$err10"
else
  skip "pwd-fallback-no-project-dir" "git not available"
fi

# --- Case 11: CLAUDE_PLUGIN_ROOT unset does not crash the hook ---
tmp11="$(mktemp -d)"
if assert_no_git_repo "$tmp11"; then
  out11="$(mktemp)"; err11="$(mktemp)"
  run_capture "$out11" "$err11" env -u CLAUDE_PLUGIN_ROOT CLAUDE_PROJECT_DIR="$tmp11" bash "$hook_script"
  exit11=$?
  assert_output_exact "plugin-root-unset-no-crash" "$out11" "$exit11" "$ask_line"
  rm -f "$out11" "$err11"
else
  skip "plugin-root-unset-no-crash" "temp dir unexpectedly resolves inside a git repo"
fi
rm -rf "$tmp11"

echo ""
echo "Results: ${pass_count} passed, ${fail_count} failed, ${skip_count} skipped"

if [ "$fail_count" -ne 0 ]; then
  exit 1
fi
exit 0
