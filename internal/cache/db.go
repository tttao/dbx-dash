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

const schemaVersion = 2

// migrations is the ordered list of DDL steps, one per schema version.
// Index 0 = v1, index 1 = v2, etc.
var migrations = []string{
	// v1 — jobs and clusters
	`
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
);`,
	// v2 — workspace identity (groups with members, users, service principals)
	`
CREATE TABLE IF NOT EXISTS identity_cache (
    workspace   TEXT NOT NULL PRIMARY KEY,
    groups_json TEXT NOT NULL DEFAULT '[]',
    fetched_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`,
}

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
	for ver < schemaVersion {
		step := migrations[ver] // ver is 0-based index into migrations
		if _, err := db.Exec(step); err != nil {
			return fmt.Errorf("migrate to v%d: %w", ver+1, err)
		}
		ver++
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", ver)); err != nil {
			return fmt.Errorf("set schema version %d: %w", ver, err)
		}
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
