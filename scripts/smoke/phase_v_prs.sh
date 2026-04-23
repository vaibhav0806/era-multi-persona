#!/usr/bin/env bash
# Phase V smoke: PR creation wiring, reject PR-close order, pr_number persistence.
# Live PR round-trip verified manually via /task to a real sandbox repo.
set -euo pipefail
go test -race -count=1 -run 'TestQueue_RunNext_OpensPROnSuccess|TestQueue_RunNext_PR|TestQueue_RejectTask_|TestComposePRBody_|TestTruncate_|TestRepo_SetPRNumber_' \
    ./internal/queue/... ./internal/db/... > /dev/null
echo "OK: phase V — PR wiring + reject reorder + pr_number persistence all green"
