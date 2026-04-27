-- +goose Up
CREATE TABLE personas (
  token_id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  owner_addr TEXT NOT NULL,
  system_prompt_uri TEXT NOT NULL,
  ens_subname TEXT,
  description TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX personas_name_idx ON personas(name);

-- +goose Down
DROP INDEX IF EXISTS personas_name_idx;
DROP TABLE IF EXISTS personas;
