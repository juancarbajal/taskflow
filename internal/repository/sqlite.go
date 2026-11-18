package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "modernc.org/sqlite"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending', -- pending | in_progress | review | done
    delegable_to_ai BOOLEAN NOT NULL DEFAULT 1,
    tags TEXT DEFAULT '[]', -- Array JSON serializado (ej. ["go", "docs"])
    assigned_agent TEXT DEFAULT '',
    ai_notes TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_ai ON tasks(status, delegable_to_ai);
`

// NewSQLiteDB opens a SQLite database using the pure-Go modernc.org/sqlite driver,
// configures WAL mode and busy_timeout=5000 across all pooled connections, and runs
// the initial idempotent schema migration.
func NewSQLiteDB(dbPath string) (*sql.DB, error) {
	if strings.TrimSpace(dbPath) == "" {
		dbPath = "taskflow.db"
	}

	dsn := buildSQLiteDSN(dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("repository.NewSQLiteDB open: %w", err)
	}

	ctx := context.Background()
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA foreign_keys = ON;",
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("repository.NewSQLiteDB pragma (%s): %w", pragma, err)
		}
	}

	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("repository.NewSQLiteDB migrate: %w", err)
	}

	return db, nil
}

func buildSQLiteDSN(dbPath string) string {
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "foreign_keys(1)")

	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	return dbPath + sep + q.Encode()
}
