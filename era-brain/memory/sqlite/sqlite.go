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
	// SQLite serializes writes through a single connection lock; cap pool to 1
	// so the database/sql layer queues writes instead of racing on SQLITE_BUSY.
	// This satisfies memory.Provider's "safe for concurrent use" contract.
	db.SetMaxOpenConns(1)
	return &Provider{db: db}, nil
}

// Close closes the underlying database.
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

// PutKV inserts a new row; GetKV reads the latest by seq.
// Row count grows without bound — acceptable for reference impl at era-brain scale.
// The 0G KV impl (M7-B) will use native upsert and will not need this seq-scan pattern.
func (p *Provider) PutKV(ctx context.Context, ns, key string, val []byte) error {
	if _, err := p.db.ExecContext(ctx,
		`INSERT INTO entries(namespace, key, val, is_kv) VALUES(?,?,?,1)`,
		ns, key, val); err != nil {
		return fmt.Errorf("putkv: %w", err)
	}
	return nil
}

func (p *Provider) AppendLog(ctx context.Context, ns string, entry []byte) error {
	// key is intentionally empty for log entries; the is_kv flag separates rows
	// at query time, but the empty key keeps Log entries from accidentally
	// matching a real KV (key, ns) pair.
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
	out := make([][]byte, 0)
	for rows.Next() {
		var v []byte
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
