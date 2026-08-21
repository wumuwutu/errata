package hooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanStaleSessions(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	stale := filepath.Join(dir, "sess-111.err")
	fresh := filepath.Join(dir, "sess-222.err")
	other := filepath.Join(dir, "unrelated.txt")
	for _, f := range []string{stale, fresh, other} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := now.Add(-2 * SessionMaxAge)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	// An ancient non-sess file must be left alone.
	if err := os.Chtimes(other, old, old); err != nil {
		t.Fatal(err)
	}

	if err := CleanStaleSessions(dir, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale sess file should be removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("fresh sess file should survive")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("unrelated file should survive")
	}
}

func TestCleanStaleSessionsMissingDir(t *testing.T) {
	if err := CleanStaleSessions(filepath.Join(t.TempDir(), "nope"), time.Now()); err == nil {
		t.Fatal("want error for missing dir")
	}
}
