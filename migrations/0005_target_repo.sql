-- migrations/0005_target_repo.sql
-- Add per-task target repo. NULL/empty means "use orchestrator default
-- (PI_GITHUB_SANDBOX_REPO)". Populated when user sends /task owner/repo desc.

-- +goose Up
ALTER TABLE tasks ADD COLUMN target_repo TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite can drop columns since 3.35, but we skip for consistency with prior migrations.
SELECT 1;
