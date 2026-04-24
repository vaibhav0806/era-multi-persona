#!/usr/bin/env bash
# Phase AE smoke: backup script syntax + rclone template validity + install.sh updates.
set -euo pipefail

# Shell syntax check
bash -n deploy/era-backup.sh

# rclone template has the expected placeholders
grep -q "YOUR_B2_ACCOUNT_ID"      deploy/rclone.conf.template
grep -q "YOUR_B2_APPLICATION_KEY" deploy/rclone.conf.template

# install.sh includes rclone in the apt list
grep -q "rclone"                  deploy/install.sh

echo "OK: phase AE — backup script + rclone template + install.sh all valid"
