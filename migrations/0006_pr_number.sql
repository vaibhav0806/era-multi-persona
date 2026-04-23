-- +goose Up
ALTER TABLE tasks ADD COLUMN pr_number INTEGER;

-- +goose Down
SELECT 1;
