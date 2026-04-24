#!/usr/bin/env bash
# Phase Z smoke: Bearer auth is in place, env template drift is cleared,
# gitignore protects stray binaries.
set -euo pipefail

# Bearer assertion in githubpr tests still green
go test -race -count=1 -run 'TestDefaultBranch|TestCreate_|TestClose_' \
    ./internal/githubpr/... > /dev/null

# env template is clean
if grep -q PI_GITHUB_APP_PRIVATE_KEY_PATH deploy/env.template; then
    echo "FAIL: PI_GITHUB_APP_PRIVATE_KEY_PATH still present in env.template"
    exit 1
fi

# gitignore protects stray binaries
grep -qE '^/runner$'  .gitignore || { echo "FAIL: /runner not in .gitignore"; exit 1; }
grep -qE '^/sidecar$' .gitignore || { echo "FAIL: /sidecar not in .gitignore"; exit 1; }

echo "OK: phase Z — cleanup batch all checks green"
