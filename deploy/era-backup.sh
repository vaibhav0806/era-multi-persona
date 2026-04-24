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

# --- M5: offsite push to B2 ---
if [ -f /etc/era/rclone.conf ] && command -v rclone >/dev/null 2>&1; then
    rclone --config=/etc/era/rclone.conf copy \
        "$OUTDIR/pi-agent-$STAMP.db.gz" b2:era-backups/ \
        --log-level INFO 2>&1 | tee -a /var/log/era-backup.log
else
    echo "$(date -Is) rclone/config missing; skipping offsite push" >> /var/log/era-backup.log
fi
