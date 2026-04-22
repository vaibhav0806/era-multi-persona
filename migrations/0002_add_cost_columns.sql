-- migrations/0002_add_cost_columns.sql
-- +goose Up
ALTER TABLE tasks ADD COLUMN tokens_used INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN cost_cents  INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite can't DROP COLUMN cleanly until 3.35; skipping down for M1.
-- Rolling back = delete file + restore from migration 0001 state by hand.
SELECT 1;
