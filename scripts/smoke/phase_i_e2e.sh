#!/usr/bin/env bash
# Phase I smoke: full Go test suite + all 3 E2E tests against real services.
# Requires .env populated with all M0 + M1 values.
set -euo pipefail
set -a; source .env; set +a
go test -race -count=1 ./... > /dev/null
go test -tags e2e -count=1 -timeout 8m ./internal/e2e/... > /dev/null
echo "OK: phase I — full suite + 3 E2E tests (M0 dummy-task path, M1 success, M1 cap-abort) all green"
