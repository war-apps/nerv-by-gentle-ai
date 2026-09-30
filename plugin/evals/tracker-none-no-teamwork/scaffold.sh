#!/usr/bin/env bash
# Seeds an activated repo with tasks.provider: none and git.worktree:
# never, matching bench/journeys.md's J5 "tracker none" variant. Runs
# only under `claude plugin eval --scaffold`.
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

cat > src/greeter.py <<'EOF'
def greet(name):
    return f"Hello, {name}!"
EOF

git add -A
git -c user.name=eval -c user.email=eval@example.com commit -q -m "scaffold"
