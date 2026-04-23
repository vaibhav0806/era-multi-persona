#!/usr/bin/env bash
# Phase W smoke: running-task cancel mechanics (RunningSet, CancelTask running
# path, RunNext killed path, Reconcile). Live docker-kill round-trip verified
# manually.
set -euo pipefail
go test -race -count=1 -run 'TestRunningSet_|TestCancelTask_|TestQueue_RunNext_KilledTask_|TestReconcile_|TestBuildDockerArgs_' \
    ./internal/queue/... ./internal/runner/... > /dev/null
echo "OK: phase W — running-task cancel all unit tests green"
