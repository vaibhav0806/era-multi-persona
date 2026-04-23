#!/usr/bin/env bash
# Phase J smoke: cmd/sidecar binary builds for linux/amd64, image runs the
# sidecar before the runner, and the audit middleware emits an AUDIT line
# per HTTP request (verified via the entrypoint's /health probe).
set -euo pipefail
make sidecar-linux > /dev/null
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/era-sidecar ./cmd/sidecar > /dev/null
file /tmp/era-sidecar | grep -q "ELF 64-bit"
rm /tmp/era-sidecar
make docker-runner > /dev/null
out=$(docker run --rm \
    -e ERA_TASK_ID=999 -e ERA_TASK_DESCRIPTION=t \
    -e ERA_GITHUB_PAT=x -e ERA_GITHUB_REPO=x/y \
    -e ERA_OPENROUTER_API_KEY=x -e ERA_PI_MODEL=x \
    -e ERA_MAX_TOKENS=1 -e ERA_MAX_COST_CENTS=1 -e ERA_MAX_ITERATIONS=1 -e ERA_MAX_WALL_SECONDS=10 \
    era-runner:m2 2>&1 || true)
echo "$out" | grep -q "sidecar ready" || { echo "FAIL: sidecar did not start"; exit 1; }
echo "$out" | grep -q '^AUDIT ' || { echo "FAIL: no AUDIT line emitted"; exit 1; }
echo "OK: phase J — sidecar builds, runs in container, emits AUDIT lines"
