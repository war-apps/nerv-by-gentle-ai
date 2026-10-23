#!/usr/bin/env bash
# Seeds an activated repo with one README carrying a single typo, so the
# NERV classification rules (SKILL.md "Classification: DIRECT, LIGHT or
# FULL") resolve unambiguously to DIRECT: one mechanical, already-understood
# file, no research, no design decision, no critical_paths entry, no
# skill/script/command. Runs only under `claude plugin eval --scaffold`.
set -euo pipefail

git init -q -b main

mkdir -p .nerv src
cat > .ai-workflow.yml <<'YAML'
enabled: true
git:
  worktree: never
  base_branch: main
tasks:
  provider: none
  ask_when_missing: false
YAML

cat > README.md <<'MD'
# Greeter

Call `greet(name)` to recieve a friendly greeting.
MD

cat > src/greeter.py <<'PY'
def greet(name):
    return f"Hello, {name}!"
PY

git add -A
git -c user.name=eval -c user.email=eval@example.com commit -q -m "scaffold"
