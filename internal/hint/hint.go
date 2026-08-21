// Package hint renders the restrained gray notice shown when a captured
// error has been seen before (dev-guide §7.6: at most two lines, gray,
// never steal the show).
package hint

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wumuwutu/dejavu/internal/store"
)

const (
	gray  = "\x1b[90m"
	reset = "\x1b[0m"

	maxSolutionRunes = 100
)

// Print writes the hit hint for rec to w (typically os.Stderr).
// rec is the matching record; similar marks a degraded "similar error"
// match (Hamming distance within threshold) rather than an exact hit.
func Print(w io.Writer, rec *store.Error, similar bool) {
	if rec == nil {
		return
	}
	when := rec.FirstSeen.Format("2006-01-02")
	where := shortenHome(rec.ProjectDir)

	var b strings.Builder
	b.WriteString(gray)
	if similar {
		fmt.Fprintf(&b, "── err ── 相似错误：你于 %s 在 %s 见过类似的（err show %d 查看）", when, where, rec.ID)
	} else {
		fmt.Fprintf(&b, "── err ── 你于 %s 在 %s 遇到过此错误（第%d次）", when, where, rec.Count)
		if rec.Solution != "" {
			fmt.Fprintf(&b, "\n   解法：%s（err show %d 查看详情）", truncate(rec.Solution), rec.ID)
		} else {
			fmt.Fprintf(&b, "\n   见过但还没记录解法（err fix 记录 | err show %d 查看）", rec.ID)
		}
	}
	b.WriteString(reset)
	fmt.Fprintln(w, b.String())
}

// PrintSolved writes the one-line "did you just fix this?" nudge after a
// successful command near a fresh pending error (dev-guide §7.2
// DETECTED_SUCCESS). Gray, single line, never pushy.
func PrintSolved(w io.Writer, e *store.Error) {
	if e == nil {
		return
	}
	fmt.Fprintf(w, "%s── err ── 刚才的 %s 好像解决了？err fix 记录解法%s\n",
		gray, truncate(e.Signature), reset)
}

func shortenHome(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if strings.HasPrefix(dir, home+string(os.PathSeparator)) {
		return "~" + dir[len(home):]
	}
	return dir
}

func truncate(s string) string {
	s = strings.Join(strings.Fields(s), " ") // collapse newlines/whitespace
	r := []rune(s)
	if len(r) <= maxSolutionRunes {
		return s
	}
	return string(r[:maxSolutionRunes-1]) + "…"
}
