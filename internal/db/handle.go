package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/vaibhav0806/pi-agent/migrations"
)

type Handle struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, applies any pending
// migrations, and returns a Handle. Enables WAL mode and foreign keys.
func Open(ctx context.Context, path string) (*Handle, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("goose dialect: %w", err)
	}
	// With SetBaseFS, the dir argument is the path within the embed FS.
	// Our migrations package embeds *.sql at its root, so use ".".
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("goose up: %w", err)
	}

	return &Handle{db: sqlDB}, nil
}

func (h *Handle) Raw() *sql.DB { return h.db }
func (h *Handle) Close() error { return h.db.Close() }
