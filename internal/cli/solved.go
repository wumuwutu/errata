package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/hint"
	"github.com/wumuwutu/dejavu/internal/store"
)

// remindInterval bounds the success nudge to once per error per 24h
// (restraint red line, dev-guide §9).
const remindInterval = 24 * time.Hour

// solvedHint implements DETECTED_SUCCESS (dev-guide §7.2), corrected by
// real-world feedback: any success used to nudge, which cried wolf on
// unrelated commands. Now the successful command's program must match the
// pending error's program (python3 == python) — a fix almost always
// re-runs the same tool. One store open, one query; every failure silent.
func solvedHint(dir, command string, out io.Writer) {
	if dir == "" {
		return
	}
	cfg, _ := config.Load()
	window := time.Duration(config.DefaultSuccessWindowMinutes) * time.Minute
	if cfg != nil {
		if cfg.SuccessWindowMinutes <= 0 {
			return // success detection disabled
		}
		window = time.Duration(cfg.SuccessWindowMinutes) * time.Minute
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return
	}
	if _, err := os.Stat(dbPath); err != nil {
		return // no database yet: nothing pending; don't create one here
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return
	}
	defer st.Close() //nolint:errcheck

	now := time.Now()
	candidates, err := st.RecentPendingInDir(dir, now, window, remindInterval)
	if err != nil {
		return
	}
	prog := programOf(command)
	for i := range candidates {
		if programOf(candidates[i].Command) != prog {
			continue // unrelated program: no nudge
		}
		st.MarkReminded(candidates[i].ID, now) //nolint:errcheck
		hint.PrintSolved(out, &candidates[i])
		return
	}
}

// programOf normalizes a command line's argv[0] for comparison: skips
// leading VAR=value assignments, takes the basename, and strips any
// trailing version number, so interpreter aliases match
// (python3.11 -> python, python3 -> python, node20 -> node).
func programOf(commandLine string) string {
	base := ""
	for _, f := range strings.Fields(commandLine) {
		if isAssignment(f) {
			continue
		}
		base = filepath.Base(f)
		break
	}
	return strings.TrimRight(base, "0123456789.")
}

// isAssignment reports whether f looks like a leading VAR=value prefix.
func isAssignment(f string) bool {
	i := strings.IndexByte(f, '=')
	if i <= 0 {
		return false
	}
	for j, r := range f[:i] {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || j > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}
