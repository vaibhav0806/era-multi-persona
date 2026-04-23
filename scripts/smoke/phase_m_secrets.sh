#!/usr/bin/env bash
# Phase M smoke: verify secrets are out of the runner's env and only reach the
# sidecar. Static code check (greps) + unit tests. Live verification is the
# manual Telegram smoke (logged in phase_m_secrets evidence in git history).
set -euo pipefail

# 1. No PAT or OpenRouter key references in cmd/runner/config.go
if grep -E "GitHubPAT|OpenRouter|API_KEY|PAT" cmd/runner/config.go | grep -v '^\s*//' | grep -v '^$' | grep -q .; then
    echo "FAIL: cmd/runner/config.go still references secrets"
    exit 1
fi

# 2. docker.go passes PI_SIDECAR_* (sidecar-only), not ERA_GITHUB_PAT or ERA_OPENROUTER_API_KEY
grep -q "PI_SIDECAR_GITHUB_PAT" internal/runner/docker.go || { echo "FAIL: docker.go missing PI_SIDECAR_GITHUB_PAT"; exit 1; }
grep -q "PI_SIDECAR_OPENROUTER_API_KEY" internal/runner/docker.go || { echo "FAIL: docker.go missing PI_SIDECAR_OPENROUTER_API_KEY"; exit 1; }
if grep -q "ERA_GITHUB_PAT=" internal/runner/docker.go; then
    echo "FAIL: docker.go still passes legacy ERA_GITHUB_PAT to runner env"
    exit 1
fi
if grep -q "ERA_OPENROUTER_API_KEY=" internal/runner/docker.go; then
    echo "FAIL: docker.go still passes legacy ERA_OPENROUTER_API_KEY to runner env"
    exit 1
fi

# 3. Sidecar unit tests for the three secret-handling endpoints
go test -race -count=1 -run 'TestLLM_|TestCredentials_|TestSearch_' ./cmd/sidecar/... > /dev/null

# 4. Image builds clean
make docker-runner > /dev/null 2>&1

echo "OK: phase M — secrets isolated in sidecar; runner env has no PAT or OR key; /llm/* /credentials/git /search unit tests green"
