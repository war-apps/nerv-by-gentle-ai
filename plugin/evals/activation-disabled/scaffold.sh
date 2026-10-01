#!/usr/bin/env bash
# Seeds an empty workspace with a git repo and a disabled .nerv/nerv.yaml
# (enabled: false), so the NERV SessionStart hook must stay silent. Runs
# only under `claude plugin eval --scaffold`.
set -euo pipefail

git init -q -b main

mkdir -p .nerv
cat > .nerv/nerv.yaml <<'EOF'
enabled: false
git:
  worktree: never
  base_branch: main
tasks:
  provider: none
  ask_when_missing: false
EOF

git add -A
git -c user.name=eval -c user.email=eval@example.com commit -q -m "scaffold"
