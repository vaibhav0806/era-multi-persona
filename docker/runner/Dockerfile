FROM node:22-alpine

# git for the runner; bash + wget for the entrypoint script; iptables for
# Phase K (Phase J doesn't use it yet but installing now avoids a rebuild).
RUN apk add --no-cache git bash ca-certificates coreutils curl iptables wget

ARG PI_VERSION=latest
RUN npm install -g @mariozechner/pi-coding-agent@${PI_VERSION} \
    && pi --version > /pi-version.txt

ENV PI_CODING_AGENT_DIR=/tmp/pi-state

# Sidecar binary (Phase J)
COPY bin/era-sidecar-linux /usr/local/bin/era-sidecar
RUN chmod +x /usr/local/bin/era-sidecar

# Runner binary (M1)
COPY bin/era-runner-linux /usr/local/bin/era-runner
RUN chmod +x /usr/local/bin/era-runner

# Entrypoint that starts sidecar then runner
COPY docker/runner/entrypoint.sh /usr/local/bin/era-entrypoint
RUN chmod +x /usr/local/bin/era-entrypoint

WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/era-entrypoint"]
