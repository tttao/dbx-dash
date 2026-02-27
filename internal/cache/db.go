// Package cache provides a local SQLite-backed store for Databricks resource
// snapshots. It is used to display recent data when a workspace is offline.
// Uses modernc.org/sqlite (pure Go, no CGO required).
package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // register sqlite3 driver
)

const schemaVersion = 1

const schema = `
CREATE TABLE IF NOT EXISTS job_snapshots (
    workspace      TEXT NOT NULL,
    job_id         INTEGER NOT NULL,
    job_name       TEXT NOT NULL,
    last_run_id    INTEGER,
    last_run_state TEXT,
    last_run_result TEXT,
    duration_ms    INTEGER,
    fetched_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace, job_id)
);

CREATE TABLE IF NOT EXISTS cluster_snapshots (
    workspace    TEXT NOT NULL,
    cluster_id   TEXT NOT NULL,
    cluster_name TEXT NOT NULL,
    state        TEXT NOT NULL,
    num_workers  INTEGER NOT NULL DEFAULT 0,
    fetched_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace, cluster_id)
);
`

// Open opens (or creates) the SQLite database at the given path, initialising
// the schema if needed. Use "~" prefix for home-relative paths.
func Open(path string) (*sql.DB, error) {
	path = expandHome(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open cache db: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	var ver int
	_ = db.QueryRow("PRAGMA user_version").Scan(&ver)
	if ver >= schemaVersion {
		return nil
	}
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	return nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, path[2:])
		}
	}
	return path
}
