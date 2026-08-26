package hooks

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SessionMaxAge is how long per-session buffer files are kept. Hook shells
// come and go; without cleanup sess-* files would pile up forever.
const SessionMaxAge = 7 * 24 * time.Hour

// SessionEnvVar carries the hooked shell's session id (its PID) to child
// processes. `err fix` uses it to find this session's command log; unset
// means no hooked session (err run, pipes) and no solution draft.
const SessionEnvVar = "ERRATA_SESSION"

// SessionsDir returns the per-user runtime directory the hook scripts keep
// session buffers in. Must match __errata_dir in scripts/errata.{bash,zsh}.
func SessionsDir() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.Getenv("TMPDIR")
	}
	if dir == "" {
		dir = "/tmp"
	}
	return filepath.Join(dir, "errata-"+strconv.Itoa(os.Getuid()))
}

// CommandsLogPath returns the session's command-log file (sess-<id>.cmds,
// written by the hook for err fix drafts). ok is false when the id is
// empty or not all digits — it comes from the environment, so it is
// validated before it ever touches a path.
func CommandsLogPath(sessionID string) (path string, ok bool) {
	if sessionID == "" {
		return "", false
	}
	for _, r := range sessionID {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return filepath.Join(SessionsDir(), "sess-"+sessionID+".cmds"), true
}

// CleanStaleSessions removes sess-* buffer files (.err/.fifo/.cmds) older
// than SessionMaxAge from dir — the sess- prefix covers them all. Errors
// are returned to the caller, which treats cleanup as best-effort (it must
// never affect recording).
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
