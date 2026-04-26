// Package sqlite is a SQLite-backed reference impl of memory.Provider.
// Used as the default in M7-A; supplanted (but never removed) by 0G impls in M7-B.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS entries (
  seq       INTEGER PRIMARY KEY AUTOINCREMENT,
  namespace TEXT NOT NULL,
  key       TEXT NOT NULL,
  val       BLOB NOT NULL,
  is_kv     INTEGER NOT NULL CHECK (is_kv IN (0,1))
);
CREATE INDEX IF NOT EXISTS idx_entries_kv ON entries(namespace, key) WHERE is_kv = 1;
CREATE INDEX IF NOT EXISTS idx_entries_log ON entries(namespace, seq) WHERE is_kv = 0;
`

// Provider is a memory.Provider backed by SQLite.
type Provider struct {
	db *sql.DB
}

// Open creates or opens a SQLite database at path. Caller must Close.
func Open(path string) (*Provider, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Provider{db: db}, nil
}

func (p *Provider) Close() error { return p.db.Close() }

func (p *Provider) GetKV(ctx context.Context, ns, key string) ([]byte, error) {
	var val []byte
	err := p.db.QueryRowContext(ctx,
		`SELECT val FROM entries WHERE namespace = ? AND key = ? AND is_kv = 1
		 ORDER BY seq DESC LIMIT 1`, ns, key).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, memory.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getkv: %w", err)
	}
	return val, nil
}

func (p *Provider) PutKV(ctx context.Context, ns, key string, val []byte) error {
	if _, err := p.db.ExecContext(ctx,
		`INSERT INTO entries(namespace, key, val, is_kv) VALUES(?,?,?,1)`,
		ns, key, val); err != nil {
		return fmt.Errorf("putkv: %w", err)
	}
	return nil
}

func (p *Provider) AppendLog(ctx context.Context, ns string, entry []byte) error {
	if _, err := p.db.ExecContext(ctx,
		`INSERT INTO entries(namespace, key, val, is_kv) VALUES(?,'',?,0)`,
		ns, entry); err != nil {
		return fmt.Errorf("appendlog: %w", err)
	}
	return nil
}

func (p *Provider) ReadLog(ctx context.Context, ns string) ([][]byte, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT val FROM entries WHERE namespace = ? AND is_kv = 0 ORDER BY seq ASC`, ns)
	if err != nil {
		return nil, fmt.Errorf("readlog: %w", err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var v []byte
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
