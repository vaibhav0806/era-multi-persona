#!/usr/bin/env bash
# Phase AI smoke: completion_message_id round-trip + ComposeReplyPrompt + handler reply detection.
set -euo pipefail
go test -race -count=1 -run 'TestRepo_CompletionMessageID_|TestComposeReplyPrompt_|TestHandler_Reply|TestFakeClient_' \
    ./internal/db/... ./internal/replyprompt/... ./internal/telegram/... > /dev/null
echo "OK: phase AI — reply threading all unit tests green"
