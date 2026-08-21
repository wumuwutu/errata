package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionMaxAge is how long per-session buffer files are kept. Hook shells
// come and go; without cleanup sess-* files would pile up forever.
const SessionMaxAge = 7 * 24 * time.Hour

// CleanStaleSessions removes sess-* buffer files older than SessionMaxAge
// from dir. Errors are returned to the caller, which treats cleanup as
// best-effort (it must never affect recording).
func CleanStaleSessions(dir string, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "sess-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // raced with another cleanup; harmless
		}
		if now.Sub(info.ModTime()) > SessionMaxAge {
			os.Remove(filepath.Join(dir, e.Name())) //nolint:errcheck // best-effort
		}
	}
	return nil
}
