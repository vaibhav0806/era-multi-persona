#!/usr/bin/env bash
# Nightly SQLite backup. Runs as root via /etc/cron.d/era-backup.
# Writes /var/backups/era/pi-agent-YYYYMMDD.db.gz, prunes files older than 7d.
set -euo pipefail
DB=/opt/era/pi-agent.db
OUTDIR=/var/backups/era
STAMP=$(date +%Y%m%d)
TMP=$(mktemp)
trap "rm -f $TMP" EXIT
sqlite3 "$DB" ".backup $TMP"
gzip -c "$TMP" > "$OUTDIR/pi-agent-$STAMP.db.gz"
chown era:era "$OUTDIR/pi-agent-$STAMP.db.gz"
find "$OUTDIR" -name 'pi-agent-*.db.gz' -mtime +7 -delete
