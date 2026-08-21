package cli

import (
	"io"
	"os"
	"time"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/hint"
	"github.com/wumuwutu/dejavu/internal/store"
)

// remindInterval bounds the success nudge to once per error per 24h
// (restraint red line, dev-guide §9).
const remindInterval = 24 * time.Hour

// solvedHint implements DETECTED_SUCCESS (dev-guide §7.2): a successful
// command in a directory with a fresh pending error earns exactly one gray
// line. One store open, one query — the prompt path stays cheap. Every
// failure is silent: this runs on the prompt path and must never disturb.
func solvedHint(dir string, out io.Writer) {
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
	e, err := st.RecentPendingInDir(dir, now, window, remindInterval)
	if err != nil || e == nil {
		return
	}
	st.MarkReminded(e.ID, now) //nolint:errcheck
	hint.PrintSolved(out, e)
}
