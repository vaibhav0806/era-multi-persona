#!/usr/bin/env bash
# Phase O smoke: GitHub App is the only credential path. PAT references
# removed from required config. JWT mint + installation token exchange
# verified via unit tests. Full pipeline verified via e2e.
set -euo pipefail

# 1. No code path reads PI_GITHUB_PAT (comments + sidecar's PI_SIDECAR_GITHUB_PAT are fine)
if grep -rn 'os.Getenv("PI_GITHUB_PAT")' --include='*.go' . 2>/dev/null; then
    echo "FAIL: production code still reads PI_GITHUB_PAT"
    exit 1
fi

# 2. Config enforces App credentials
grep -q "PI_GITHUB_APP_ID is required" internal/config/config.go \
    || { echo "FAIL: config doesn't enforce PI_GITHUB_APP_ID"; exit 1; }

# 3. githubapp unit tests green
go test -race -count=1 ./internal/githubapp/... > /dev/null

# 4. Bad-env smoke proves App vars required
mv .env .env.bak
out=$(env -i PATH=/usr/bin:/bin ./bin/orchestrator 2>&1 || true)
mv .env.bak .env
echo "$out" | grep -q "is required" || { echo "FAIL: orchestrator didn't error on missing env"; exit 1; }

echo "OK: phase O — GitHub App is the only credential path; PAT removed from required config"
