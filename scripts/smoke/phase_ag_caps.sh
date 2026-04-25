#!/usr/bin/env bash
# Phase AG smoke: budget profile lib + flag parser + CreateTask cascade.
set -euo pipefail
go test -race -count=1 -run 'TestProfiles_|TestParseBudgetFlag_|TestQueue_RunNext_DeepProfilePassesThroughCaps|TestBuildDockerArgs_PerTaskCaps' \
    ./internal/budget/... ./internal/queue/... ./internal/runner/... > /dev/null
echo "OK: phase AG — caps + budget profiles unit tests green"
