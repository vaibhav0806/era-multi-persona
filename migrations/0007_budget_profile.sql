-- +goose Up
ALTER TABLE tasks ADD COLUMN budget_profile TEXT NOT NULL DEFAULT 'default';

-- +goose Down
SELECT 1;
