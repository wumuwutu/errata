package store

import (
	"database/sql"
	"fmt"
)

// migrations is the ordered schema history: entry i upgrades the database
// from version i to version i+1 (dev-guide §16.5: user data surviving
// upgrades is a hard requirement, so never edit or reorder existing
// entries — append only). Migration 1 is the v0.1.0 schema; it is written
// entirely with IF NOT EXISTS so a pre-migration v0.1.0 database (tables
// present, no schema_version) upgrades in place without touching data.
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS errors (
	  id INTEGER PRIMARY KEY,
	  fingerprint TEXT UNIQUE,
	  signature TEXT,
	  raw_sample TEXT,
	  language TEXT,
	  command TEXT,
	  project_dir TEXT,
	  git_commit TEXT,
	  runtime TEXT,
	  os TEXT,
	  created_at TIMESTAMP,
	  first_seen TIMESTAMP,
	  last_seen TIMESTAMP,
	  count INTEGER DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS fixes (
	  id INTEGER PRIMARY KEY,
	  error_id INTEGER REFERENCES errors(id),
	  solution TEXT,
	  draft TEXT,
	  commands_between TEXT,
	  git_diff_ref TEXT,
	  created_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS pending (
	  id INTEGER PRIMARY KEY,
	  error_id INTEGER REFERENCES errors(id),
	  detected_at TIMESTAMP,
	  status TEXT
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS errors_fts USING fts5(signature, solution);

	CREATE INDEX IF NOT EXISTS idx_pending_status ON pending(status);
	CREATE INDEX IF NOT EXISTS idx_fixes_error ON fixes(error_id);

	CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL);`,

	// 2: success-detection reminders (dev-guide §7.2 DETECTED_SUCCESS) —
	// one nudge per error per 24h needs a persisted "last reminded" mark.
	`ALTER TABLE pending ADD COLUMN reminded_at TIMESTAMP;`,

	// 3: err delete/clear must never lead to reused ids (a fresh record
	// wearing an old id would inherit that id's mental history). Rebuild
	// errors with AUTOINCREMENT so SQLite keeps the high-water mark in
	// sqlite_sequence. Data is copied verbatim; errors_fts keys off
	// rowid (= errors.id) and stays valid.
	`CREATE TABLE errors_new (
	  id INTEGER PRIMARY KEY AUTOINCREMENT,
	  fingerprint TEXT UNIQUE,
	  signature TEXT,
	  raw_sample TEXT,
	  language TEXT,
	  command TEXT,
	  project_dir TEXT,
	  git_commit TEXT,
	  runtime TEXT,
	  os TEXT,
	  created_at TIMESTAMP,
	  first_seen TIMESTAMP,
	  last_seen TIMESTAMP,
	  count INTEGER DEFAULT 1
	);
	INSERT INTO errors_new SELECT id, fingerprint, signature, raw_sample,
	  language, command, project_dir, git_commit, runtime, os,
	  created_at, first_seen, last_seen, count FROM errors;
	DROP TABLE errors;
	ALTER TABLE errors_new RENAME TO errors;`,
}

// migrate brings the database to the latest schema version, one migration
// per transaction so a failed step leaves the previous version intact.
func migrate(db *sql.DB) error {
	version, err := schemaVersion(db)
	if err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(`DELETE FROM schema_version`); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, i+1); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// schemaVersion returns the current schema version: 0 for a database that
// predates the schema_version table (or is empty).
func schemaVersion(db *sql.DB) (int, error) {
	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'schema_version'`).Scan(&name)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var version int
	err = db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&version)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return version, err
}
