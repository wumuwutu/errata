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
	if !cfg.DraftEnabled {
		t.Fatal("draft_enabled should default to true")
	}
}

func TestDraftEnabledConfigurable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgDir := filepath.Join(dir, appName)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"),
		[]byte("draft_enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DraftEnabled {
		t.Fatal("draft_enabled: false not honored")
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
