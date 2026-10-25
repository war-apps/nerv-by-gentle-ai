#!/usr/bin/env bash
# Seeds an activated repo with one tiny single-domain source file, so the
# NERV classification rules (SKILL.md "Classification: DIRECT, LIGHT or FULL")
# resolve unambiguously to LIGHT: one pilot domain (backend), no
# critical_paths entry, no skill/script/command, but a new function
# that needs its own test, so it is not DIRECT's one mechanical file. Runs only
# under `claude plugin eval --scaffold`.
set -euo pipefail

git init -q -b main

mkdir -p .nerv src
mkdir -p .ai-workflow && cat > .ai-workflow/config.yaml <<'EOF'
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
