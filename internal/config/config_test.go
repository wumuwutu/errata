package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIgnoreRoundtrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := AddIgnore(IgnoreCommand, "npm"); err != nil {
		t.Fatal(err)
	}
	if err := AddIgnore(IgnoreDir, "~/work/secrets"); err != nil {
		t.Fatal(err)
	}
	// Duplicate add is a no-op.
	if err := AddIgnore(IgnoreCommand, "npm"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.IgnoreCommands) != 1 || cfg.IgnoreCommands[0] != "npm" {
		t.Fatalf("commands = %v", cfg.IgnoreCommands)
	}
	if len(cfg.IgnoreDirs) != 1 {
		t.Fatalf("dirs = %v", cfg.IgnoreDirs)
	}
	if !cfg.HintEnabled {
		t.Fatal("hint.enabled should default to true")
	}

	if !cfg.IsIgnored("npm", "/anywhere") || !cfg.IsIgnored("/usr/bin/npm", "/anywhere") {
		t.Fatal("command basename not ignored")
	}
	if cfg.IsIgnored("node", "/anywhere") {
		t.Fatal("node should not be ignored")
	}

	removed, err := RemoveIgnore(IgnoreCommand, "npm")
	if err != nil || !removed {
		t.Fatalf("RemoveIgnore: removed=%v err=%v", removed, err)
	}
	removed, err = RemoveIgnore(IgnoreCommand, "npm")
	if err != nil || removed {
		t.Fatalf("second RemoveIgnore should report not found")
	}
}

func TestIsIgnoredDirPrefix(t *testing.T) {
	cfg := &Config{IgnoreDirs: []string{"/srv/secret"}}
	if !cfg.IsIgnored("make", "/srv/secret") {
		t.Fatal("exact dir should match")
	}
	if !cfg.IsIgnored("make", "/srv/secret/sub") {
		t.Fatal("subdir should match")
	}
	if cfg.IsIgnored("make", "/srv/secretary") {
		t.Fatal("prefix without separator must not match")
	}
}

func TestLoadMissingConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("missing config must yield defaults: %v", err)
	}
	if cfg == nil || !cfg.HintEnabled {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.ArchiveAfterDays != DefaultArchiveAfterDays {
		t.Fatalf("ArchiveAfterDays = %d, want %d", cfg.ArchiveAfterDays, DefaultArchiveAfterDays)
	}
}

func TestArchiveAfterDaysConfigurable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgDir := filepath.Join(dir, appName)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"),
		[]byte("archive_after_days: 7\nhint:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ArchiveAfterDays != 7 {
		t.Fatalf("ArchiveAfterDays = %d, want 7", cfg.ArchiveAfterDays)
	}
	if cfg.HintEnabled {
		t.Fatal("hint.enabled: false not honored")
	}
}

// TestLegacyDirMigration: data and config directories from before the
// rename are renamed on first use, and the database file follows.
func TestLegacyDirMigration(t *testing.T) {
	dataHome := t.TempDir()
	confHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", confHome)

	// Fabricate the pre-rename layout: legacy dirs + legacy db name.
	oldData := filepath.Join(dataHome, legacyAppName)
	oldConf := filepath.Join(confHome, legacyAppName)
	if err := os.MkdirAll(oldData, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(oldConf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldData, legacyAppName+".db"), []byte("db-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldData, legacyAppName+".db-wal"), []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldConf, "config.yaml"), []byte("archive_after_days: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(dataHome, appName) {
		t.Fatalf("DataDir = %q, want the new path", dir)
	}
	if _, err := os.Stat(oldData); !os.IsNotExist(err) {
		t.Fatal("legacy data dir must be gone after rename")
	}
	db, err := DBPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(db) != "errata.db" {
		t.Fatalf("DBPath = %q, want errata.db", db)
	}
	if b, _ := os.ReadFile(db); string(b) != "db-bytes" {
		t.Fatal("database content lost in migration")
	}
	if _, err := os.Stat(db + "-wal"); err != nil {
		t.Fatal("WAL sidecar not carried over")
	}

	cfgDir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if cfgDir != filepath.Join(confHome, appName) {
		t.Fatalf("ConfigDir = %q, want the new path", cfgDir)
	}
	cfg, err := Load()
	if err != nil || cfg.ArchiveAfterDays != 7 {
		t.Fatalf("config lost in migration: %+v, %v", cfg, err)
	}

	// Idempotent: second call is a no-op on the new layout.
	if _, err := DataDir(); err != nil {
		t.Fatal(err)
	}
}

// TestLegacyMigrationKeepsNew: when both layouts exist, the new one wins
// and the old one is left untouched (no merging, no deletion).
func TestLegacyMigrationKeepsNew(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	for _, name := range []string{appName, legacyAppName} {
		if err := os.MkdirAll(filepath.Join(dataHome, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(dataHome, appName) {
		t.Fatalf("DataDir = %q, want the existing new path", dir)
	}
	if _, err := os.Stat(filepath.Join(dataHome, legacyAppName)); err != nil {
		t.Fatal("legacy dir must be left untouched when the new one exists")
	}
}
