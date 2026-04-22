#!/usr/bin/env bash
# Phase C manual smoke: Telegram ↔ orchestrator round-trip.
# This is a reference checklist — not automated because a real Telegram bot
# is required. Follow these steps by hand each time Phase C regression is run.
#
# Prereqs:
#   - .env populated with a real PI_TELEGRAM_TOKEN (from @BotFather) and
#     PI_TELEGRAM_ALLOWED_USER_ID (your numeric Telegram user ID).
#   - bin/orchestrator built via `make build`.
#
# Steps:
#   1. rm -f ./era.db ./era.db-wal ./era.db-shm
#   2. ./bin/orchestrator &
#   3. In Telegram, send each of:
#        /task hello world          -> expect: task #1 queued
#        /status 1                  -> expect: task #1: queued
#        /list                      -> expect: #1 [queued] hello world
#        /wat                       -> expect: unknown command. try /task, /status, /list
#   4. sqlite3 ./era.db "SELECT id,description,status FROM tasks;"
#        -> expect: 1|hello world|queued
#   5. Send /status 999             -> expect: task #999 not found
#   6. kill %1 (graceful shutdown; log should show "shutting down")
#   7. ./bin/orchestrator &         -> expect: "no migrations to run"
#   8. In Telegram send /list       -> expect: task #1 still listed
#   9. (Optional) From a second Telegram account, message the bot.
#        -> expect: no reply, nothing in orchestrator log
#  10. kill %1; rm -f ./era.db*

echo "This is a reference checklist. Follow steps manually."
exit 0
