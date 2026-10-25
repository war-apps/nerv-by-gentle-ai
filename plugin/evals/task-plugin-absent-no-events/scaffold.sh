#!/usr/bin/env bash
# Seeds an activated repo with git.worktree: never and a legacy tasks:
# block, matching bench/journeys.md's J5 "task extension absent" variant. The
# eval installs only the core plugin, so task:dispatcher is unavailable and
# core must ignore the tasks: block. Runs only under
# `claude plugin eval --scaffold`.
set -euo pipefail

git init -q -b main

mkdir -p .nerv src
mkdir -p .ai-workflow && cat > .ai-workflow/config.yaml <<'YAML'
enabled: true
git:
  worktree: never
  base_branch: main
tasks:
  ask_when_missing: true
YAML

cat > src/greeter.py <<'PY'
def greet(name):
    return f"Hello, {name}!"
PY

git add -A
git -c user.name=eval -c user.email=eval@example.com commit -q -m "scaffold"
