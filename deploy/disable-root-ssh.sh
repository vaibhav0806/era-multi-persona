#!/usr/bin/env bash
# Disables root SSH login. Only run AFTER ssh era@<ip> works with your key.
# Idempotent.
set -euo pipefail
CONF=/etc/ssh/sshd_config
if ! grep -qE '^PermitRootLogin no' "$CONF"; then
    sed -i -E 's/^#?PermitRootLogin.*/PermitRootLogin no/' "$CONF"
    # Unit is "ssh" on Debian/Ubuntu, "sshd" on RHEL — try both.
    systemctl reload ssh 2>/dev/null || systemctl reload sshd
    echo "root ssh disabled. test: ssh root@<ip> should fail, ssh era@<ip> should succeed."
else
    echo "root ssh already disabled; no-op."
fi
