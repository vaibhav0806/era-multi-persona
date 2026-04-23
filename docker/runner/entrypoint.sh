#!/bin/sh
# /usr/local/bin/era-entrypoint
# Starts the sidecar (as uid 100) in the background, waits for /health,
# then sets iptables rules that gate egress to sidecar-only, then execs the
# runner. Phases L+M extend the env with HTTPS_PROXY etc.
set -eu

# Sidecar listens on loopback so only in-container processes can reach it.
export PI_SIDECAR_LISTEN_ADDR="127.0.0.1:8080"

# PI_SIDECAR_OPENROUTER_API_KEY and PI_SIDECAR_GITHUB_PAT are passed in by
# the orchestrator at the container level. su -m below preserves them so the
# sidecar process sees them. (No action needed here — already in the env.)

if [ -n "${PI_SIDECAR_GITHUB_PAT:-}" ]; then
    echo "sidecar will receive GitHub PAT (${#PI_SIDECAR_GITHUB_PAT} chars)" >&2
fi
if [ -n "${PI_SIDECAR_OPENROUTER_API_KEY:-}" ]; then
    echo "sidecar will receive OpenRouter key (${#PI_SIDECAR_OPENROUTER_API_KEY} chars)" >&2
fi

# Run sidecar as uid 100. Hard-fail if adduser fails (other than "exists").
# Use -G users because gid 100 (users group) already exists in node:alpine;
# busybox adduser won't create a new primary group if the gid is taken.
if ! id sidecar >/dev/null 2>&1; then
    adduser -D -u 100 -G users sidecar || { echo "FATAL: adduser sidecar failed" >&2; exit 1; }
fi

# `su -m` preserves the calling shell's env, so PI_SIDECAR_LISTEN_ADDR
# (and PI_SIDECAR_*_API_KEY vars) reach the sidecar process.
su -m -s /bin/sh -c '/usr/local/bin/era-sidecar' sidecar &
SIDECAR_PID=$!

# Wait up to ~5s for /health.
for i in 1 2 3 4 5 10 20 30; do
    if wget -q -O - http://127.0.0.1:8080/health 2>/dev/null | grep -q "^ok$"; then
        echo "sidecar ready (pid=$SIDECAR_PID)" >&2
        break
    fi
    sleep 0.1
done

# Hard-fail if /health never returned ok within budget.
if ! wget -q -O - http://127.0.0.1:8080/health 2>/dev/null | grep -q "^ok$"; then
    echo "FATAL: sidecar failed to start within budget" >&2
    exit 1
fi

# --- Network lockdown ---
# add_rule wrapper hard-fails on any iptables error so we never exec the
# runner with a broken lockdown.
add_rule() {
    iptables "$@" || { echo "FATAL: iptables $* failed" >&2; exit 1; }
}
add_rule -I OUTPUT 1 -o lo -j ACCEPT
add_rule -A OUTPUT -p udp --dport 53 -j ACCEPT
add_rule -A OUTPUT -m owner --uid-owner 100 -j ACCEPT
add_rule -A OUTPUT -p tcp --dport 443 -j REJECT --reject-with tcp-reset
add_rule -A OUTPUT -p tcp --dport 80  -j REJECT --reject-with tcp-reset
echo "iptables lockdown active (sidecar=uid100 unrestricted)" >&2

# Tell child processes to use the sidecar as their HTTP/HTTPS proxy.
# (Pi may not honor these for its OpenAI client; that's handled in M2-15
# via custom provider config. But curl/git/npm should respect them.)
export HTTP_PROXY="http://127.0.0.1:8080"
export HTTPS_PROXY="http://127.0.0.1:8080"
export http_proxy="http://127.0.0.1:8080"
export https_proxy="http://127.0.0.1:8080"
export NO_PROXY="127.0.0.1,localhost"
export no_proxy="127.0.0.1,localhost"

# Optional diagnostic: prove allowlist works (run only when PI_SIDECAR_TEST_DIAG=1)
if [ "${PI_SIDECAR_TEST_DIAG:-}" = "1" ]; then
    echo "diag: trying allowed host (openrouter.ai)" >&2
    code=$(curl --max-time 5 -s -o /dev/null -w "%{http_code}" https://openrouter.ai/api/v1/models 2>/dev/null || echo "denied")
    echo "diag-allowed-result: $code" >&2
    echo "diag: trying disallowed host (example.com)" >&2
    code=$(curl --max-time 5 -s -o /dev/null -w "%{http_code}" https://example.com/ 2>/dev/null || echo "denied")
    echo "diag-disallowed-result: $code" >&2
fi

# Write a models.json that overrides Pi's openrouter provider baseUrl to point
# at the sidecar. This makes Pi route all LLM calls through the sidecar instead
# of hitting openrouter.ai directly. The sidecar holds the real OpenRouter key
# via PI_SIDECAR_OPENROUTER_API_KEY and injects it when proxying.
mkdir -p /tmp/pi-state
cat > /tmp/pi-state/models.json <<'MODELS_EOF'
{
  "providers": {
    "openrouter": {
      "baseUrl": "http://127.0.0.1:8080/llm/v1"
    }
  }
}
MODELS_EOF
echo "pi models.json written (openrouter -> sidecar)" >&2

# Install git credential helper that fetches creds from the sidecar.
cat > /usr/local/bin/era-git-cred <<'EOF'
#!/bin/sh
if [ "$1" = "get" ]; then
    curl -sS -X POST http://127.0.0.1:8080/credentials/git
fi
EOF
chmod +x /usr/local/bin/era-git-cred
git config --global credential.helper '/usr/local/bin/era-git-cred'
echo "git credential helper installed (/usr/local/bin/era-git-cred)" >&2

# Hand off to runner. Sidecar continues in background under uid 100.
exec /usr/local/bin/era-runner "$@"
