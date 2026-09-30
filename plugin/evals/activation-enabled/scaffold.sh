#!/usr/bin/env bash
# Seeds an empty workspace with a git repo and an activated .nerv/nerv.yaml
# (enabled: true), so the NERV SessionStart hook injects the orchestrator
# protocol. Runs only under `claude plugin eval --scaffold`.
set -euo pipefail

git init -q -b main

mkdir -p .nerv
cat > .nerv/nerv.yaml <<'EOF'
enabled: true
git:
  worktree: never
  base_branch: main
tasks:
  provider: none
  ask_when_missing: false
EOF

git add -A
git -c user.name=eval -c user.email=eval@example.com commit -q -m "scaffold"
