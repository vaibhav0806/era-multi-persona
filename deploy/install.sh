#!/usr/bin/env bash
# era install script. Run as root on a fresh Ubuntu 24.04 VPS. Idempotent.
# Prerequisites: SSH into the box works as root with your key.
# Post-install manual steps printed at the end.
set -euo pipefail

log() { echo "==> $*"; }

log "apt update + install system deps"
apt-get update -y
DEBIAN_FRONTEND=noninteractive apt-get install -y \
    docker.io docker-buildx make git rsync sqlite3 ufw curl \
    unattended-upgrades apt-listchanges

# --- Go 1.25 from official tarball (Ubuntu 24.04's golang-go is 1.22, too old) ---
GO_VER="1.25.6"
if ! command -v go &>/dev/null || [[ "$(go version | awk '{print $3}')" != "go$GO_VER" ]]; then
    log "install go $GO_VER (official tarball)"
    ARCH=$(dpkg --print-architecture)   # arm64 on CAX11, amd64 on CPX11
    curl -fsSL "https://go.dev/dl/go${GO_VER}.linux-${ARCH}.tar.gz" -o /tmp/go.tgz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tgz
    rm /tmp/go.tgz
    cat > /etc/profile.d/go.sh <<'GOEOF'
export PATH=$PATH:/usr/local/go/bin
GOEOF
    chmod 644 /etc/profile.d/go.sh
fi
export PATH=$PATH:/usr/local/go/bin

log "create era user (if missing)"
if ! id era &>/dev/null; then
    useradd -m -s /bin/bash -G docker era
fi
if [[ -f /root/.ssh/authorized_keys && ! -f /home/era/.ssh/authorized_keys ]]; then
    install -d -m 700 -o era -g era /home/era/.ssh
    install -m 600 -o era -g era /root/.ssh/authorized_keys /home/era/.ssh/authorized_keys
fi

log "create era dirs"
install -d -o era -g era /opt/era /var/backups/era
# /etc/era owned by era:era mode 700 — matches spec §3.2 and lets the era user
# read its own env/pem without sudo while still blocking any other user.
install -d -o era -g era -m 700 /etc/era

log "sudoers entry for era (from deploy/sudoers-era)"
install -m 440 /opt/era/deploy/sudoers-era /etc/sudoers.d/era
visudo -c -f /etc/sudoers.d/era >/dev/null || { echo "sudoers validation failed"; exit 1; }

log "ufw"
ufw --force reset
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw --force enable

log "unattended-upgrades"
systemctl enable --now unattended-upgrades

log "systemd unit + backup cron"
install -m 644 /opt/era/deploy/era.service /etc/systemd/system/era.service
install -m 644 /opt/era/deploy/era-backup.cron /etc/cron.d/era-backup
systemctl daemon-reload
systemctl enable era

IP=$(hostname -I | awk '{print $1}')
cat <<EOF

=== era install complete ===
Next steps (manual, from your Mac):
  1. scp .env              era@${IP}:/etc/era/env
  2. scp github-app.pem    era@${IP}:/etc/era/github-app.pem
  3. ssh era@${IP} 'chmod 600 /etc/era/env /etc/era/github-app.pem'   # /etc/era is already era:era 700
  4. scp pi-agent.db       era@${IP}:/opt/era/pi-agent.db
  5. make deploy VPS_HOST=era@${IP}                                   # from the era repo checkout on your Mac
  6. After ssh era@${IP} works: run deploy/disable-root-ssh.sh as root to lock down.
EOF
