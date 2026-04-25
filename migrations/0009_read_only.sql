-- +goose Up
ALTER TABLE tasks ADD COLUMN read_only INTEGER NOT NULL DEFAULT 0;

-- +goose Down
SELECT 1;
