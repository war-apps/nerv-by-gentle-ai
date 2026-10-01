#!/usr/bin/env bash
# Seeds an empty workspace with a plain git repo and no .nerv/nerv.yaml at
# all, so the NERV SessionStart hook has nothing to gate on and must stay
# silent. Runs only under `claude plugin eval --scaffold`.
set -euo pipefail

git init -q -b main

cat > README.md <<'EOF'
# scratch repo (no NERV marker)
EOF

git add -A
git -c user.name=eval -c user.email=eval@example.com commit -q -m "scaffold"
