#!/usr/bin/env bash
# Phase AF smoke: ci.yml exists, is valid YAML (best-effort), has the expected structure.
set -euo pipefail

test -f .github/workflows/ci.yml

# Parse as YAML when python yaml is available; otherwise skip the strict parse.
if python3 -c 'import yaml' 2>/dev/null; then
    python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
fi

# Has both jobs
grep -qE '^  test:'   .github/workflows/ci.yml
grep -qE '^  deploy:' .github/workflows/ci.yml

# Deploy is no longer gated on 'if: false'
if grep -qE '^\s+if:\s+false' .github/workflows/ci.yml; then
    echo "FAIL: deploy job still gated on 'if: false'"
    exit 1
fi

echo "OK: phase AF — ci.yml valid and deploy enabled"
