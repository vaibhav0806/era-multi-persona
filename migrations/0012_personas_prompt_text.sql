-- +goose Up
ALTER TABLE personas ADD COLUMN prompt_text TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1; -- SQLite ≤ 3.34 can't DROP COLUMN; no-op for hackathon scope
