#!/usr/bin/env bash
# Phase P smoke: diffscan rules + unified-diff parser + compare client green;
# migration 0004 applies and the new CHECK values are accepted.
set -euo pipefail
go test -race -count=1 -run 'TestRule|TestScan|TestCompare|TestRepo_SetStatus' \
    ./internal/diffscan/... ./internal/githubcompare/... ./internal/db/... > /dev/null
TMP=$(mktemp -t era.XXXXXX.db)
trap "rm -f $TMP $TMP-wal $TMP-shm" EXIT
goose -dir migrations sqlite3 "$TMP" up > /dev/null 2>&1
sqlite3 "$TMP" "INSERT INTO tasks(description,status) VALUES ('x','needs_review');"
sqlite3 "$TMP" "INSERT INTO tasks(description,status) VALUES ('y','approved');"
sqlite3 "$TMP" "INSERT INTO tasks(description,status) VALUES ('z','rejected');"
count=$(sqlite3 "$TMP" "SELECT COUNT(*) FROM tasks WHERE status IN ('needs_review','approved','rejected');")
[[ "$count" = "3" ]] || { echo "FAIL: CHECK rejected new values"; exit 1; }
echo "OK: phase P — diffscan + githubcompare + migration 0004 all verified"
