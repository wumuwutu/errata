package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/hint"
	"github.com/wumuwutu/errata/internal/store"
)

// remindInterval bounds the success nudge to once per error per 24h
// (restraint red line, dev-guide §9).
const remindInterval = 24 * time.Hour

// solvedHint implements DETECTED_SUCCESS (dev-guide §7.2), tightened twice
// by real-world feedback. v0.1.4: not every success nudges, only a success
// running the same program (python3 == python). v0.1.8: same program is not
// enough either — with several same-program pendings in one directory, any
// of them succeeding nudged the wrong error. Now the successful command
// must also share a "target" with the failed one: a non-flag argument (the
// script, the subcommand, the package) that survives flag stripping, e.g.
// `python demo7.py` failing is only matched by a succeeding command that
// also mentions demo7.py. When neither side carries a target argument, the
// program alone decides (e.g. `pip` vs `pip`). Precision first: a missed
// nudge beats a wrong one. One store open, one query; every failure silent.
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
	for i := range candidates {
		if !sameTarget(candidates[i].Command, command) {
			continue // unrelated command: no nudge
		}
		st.MarkReminded(candidates[i].ID, now) //nolint:errcheck
		hint.PrintSolved(out, &candidates[i])
		return
	}
}

// sameTarget reports whether a successful command plausibly re-ran the
// failed one: same program, plus at least one shared target argument.
// With no target arguments on either side, same program is enough.
func sameTarget(failedCmd, okCmd string) bool {
	if programOf(failedCmd) != programOf(okCmd) {
		return false
	}
	fa, oa := targetArgs(failedCmd), targetArgs(okCmd)
	if len(fa) == 0 && len(oa) == 0 {
		return true
	}
	for _, f := range fa {
		for _, o := range oa {
			if f == o {
				return true
			}
		}
	}
	return false
}

// targetArgs extracts a command line's non-flag arguments: tokens after
// the program name (itself after any leading VAR=value prefixes) that do
// not start with '-'. Quotes and directories are stripped for comparison,
// so `python "demo7.py"`, `python ./demo7.py` and `python demo7.py` share
// the target demo7.py. Flags never count as targets — but a flag's value
// (e.g. pytest in `python -m pytest`) does: the arity of arbitrary flags
// is unknowable, and treating values as targets errs toward the safer
// side here (they tend to name the thing being run).
func targetArgs(commandLine string) []string {
	fields := strings.Fields(commandLine)
	i := 0
	for i < len(fields) && isAssignment(fields[i]) {
		i++
	}
	if i < len(fields) {
		i++ // the program name itself
	}
	var out []string
	for ; i < len(fields); i++ {
		f := fields[i]
		if strings.HasPrefix(f, "-") {
			continue
		}
		f = filepath.Base(strings.Trim(f, `"'`))
		if f != "" && f != "." && f != string(os.PathSeparator) {
			out = append(out, f)
		}
	}
	return out
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
