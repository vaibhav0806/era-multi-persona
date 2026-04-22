package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/pi-agent/internal/db"
)

func TestOpen_MigratesFreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	h, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	var count int
	err = h.Raw().QueryRow(`SELECT count(*) FROM tasks`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestOpen_ReopenExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	h1, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	_, err = h1.Raw().Exec(`INSERT INTO tasks(description, status) VALUES (?, ?)`, "hi", "queued")
	require.NoError(t, err)
	require.NoError(t, h1.Close())

	h2, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h2.Close() })

	var count int
	require.NoError(t, h2.Raw().QueryRow(`SELECT count(*) FROM tasks`).Scan(&count))
	require.Equal(t, 1, count)
}
