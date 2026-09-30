#!/usr/bin/env bash
# Seeds an activated repo with one backend source file. The requested
# change (in the case prompt) also touches a new CI workflow file, so two
# pilot domains are touched (backend + ci-cd/infra) and SKILL.md's
# "Classification: LIGHT vs FULL" rules resolve unambiguously to FULL.
# Runs only under `claude plugin eval --scaffold`.
set -euo pipefail

git init -q -b main

mkdir -p .nerv src
cat > .nerv/nerv.yaml <<'EOF'
enabled: true
git:
  worktree: never
  base_branch: main
tasks:
  provider: none
  ask_when_missing: false
EOF

cat > src/calc.py <<'EOF'
def add(a, b):
    return a + b
EOF

git add -A
git -c user.name=eval -c user.email=eval@example.com commit -q -m "scaffold"
