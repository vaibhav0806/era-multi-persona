#!/usr/bin/env bash
# Phase B smoke: verify a fresh DB gets migrated and basic CRUD works through
# the orchestrator's own code path (not via the binary yet — Task 11 covers that).
# For now, this script is a reference document. It will be wired to the binary
# in later phases.

set -euo pipefail

DB=$(mktemp -t pi-smoke.XXXXXX.db)
trap "rm -f $DB $DB-wal $DB-shm" EXIT

# Direct goose run proves the migration applies on a fresh file.
goose -dir migrations sqlite3 "$DB" up
goose -dir migrations sqlite3 "$DB" status 2>&1 | grep -q "0001_init.sql"
echo "OK: migration applied to $DB"

sqlite3 "$DB" ".schema" | grep -q "CREATE TABLE tasks"
sqlite3 "$DB" ".schema" | grep -q "CREATE TABLE events"
sqlite3 "$DB" ".schema" | grep -q "CREATE TABLE approvals"
echo "OK: all 3 tables present"
