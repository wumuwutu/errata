package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateFreshDatabase(t *testing.T) {
	s := openTemp(t)
	version, err := schemaVersion(s.db)
	if err != nil {
		t.Fatal(err)
	}
	if version != len(migrations) {
		t.Fatalf("version = %d, want %d", version, len(migrations))
	}
}

func TestMigrateReopenIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "errata.db")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertError(sample("00000000000000c1")); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Reopen: migrations must be a no-op, data intact.
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	e, err := s2.FindByFingerprint("00000000000000c1")
	if err != nil || e == nil {
		t.Fatalf("data lost across reopen: %v", err)
	}
}

// TestMigrateLegacyDatabase simulates a v0.1.0 database (all tables, no
// schema_version) and checks the upgrade preserves existing rows.
func TestMigrateLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "errata.db")

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// The v0.1.0 schema, minus schema_version which did not exist yet.
	legacy := `
	CREATE TABLE errors (
	  id INTEGER PRIMARY KEY, fingerprint TEXT UNIQUE, signature TEXT,
	  raw_sample TEXT, language TEXT, command TEXT, project_dir TEXT,
	  git_commit TEXT, runtime TEXT, os TEXT,
	  created_at TIMESTAMP, first_seen TIMESTAMP, last_seen TIMESTAMP,
	  count INTEGER DEFAULT 1
	);
	CREATE TABLE fixes (
	  id INTEGER PRIMARY KEY, error_id INTEGER REFERENCES errors(id),
	  solution TEXT, draft TEXT, commands_between TEXT, git_diff_ref TEXT,
	  created_at TIMESTAMP
	);
	CREATE TABLE pending (
	  id INTEGER PRIMARY KEY, error_id INTEGER REFERENCES errors(id),
	  detected_at TIMESTAMP, status TEXT
	);
	CREATE VIRTUAL TABLE errors_fts USING fts5(signature, solution);
	INSERT INTO errors (fingerprint, signature, created_at, first_seen, last_seen)
	  VALUES ('00000000000000d1', 'TypeError: legacy', '2026-01-01T00:00:00Z',
	          '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
	`
	if _, err := db.Exec(legacy); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade legacy db: %v", err)
	}
	defer s.Close()

	version, err := schemaVersion(s.db)
	if err != nil || version != len(migrations) {
		t.Fatalf("version = %d, want %d (err=%v)", version, len(migrations), err)
	}
	e, err := s.FindByFingerprint("00000000000000d1")
	if err != nil || e == nil || e.Signature != "TypeError: legacy" {
		t.Fatalf("legacy row lost: e=%+v err=%v", e, err)
	}
}
