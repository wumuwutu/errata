// Package hint renders the restrained notices shown when a captured error
// has been seen before (dev-guide §7.6: at most two lines, faint gray,
// never steal the show). All dejavu notices share one style: a `--err--`
// prefix, faint base text, cyan command names and bright key payloads, so
// they read as system text next to the user's own terminal output. Colors
// honor NO_COLOR (see internal/termx).
package hint

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/wumuwutu/dejavu/internal/store"
	"github.com/wumuwutu/dejavu/internal/termx"
)

// maxSolutionCols bounds the solution/signature excerpt inside a hint.
const maxSolutionCols = 100

// prefix starts every hint line (ASCII only, on purpose).
const prefix = "--err--"

// Print writes the hit hint for rec to w (typically os.Stdout via the
// hook). rec is the matching record; similar marks a degraded "similar
// error" match (Hamming distance within threshold) rather than an exact
// hit. Every line starts at column 0 — the hint must never look like part
// of the command's own (possibly indented) output.
func Print(w io.Writer, rec *store.Error, similar bool) {
	if rec == nil {
		return
	}
	when := rec.FirstSeen.Format("2006-01-02")
	where := termx.ShortenHome(rec.ProjectDir)
	id := strconv.FormatInt(rec.ID, 10)

	var b strings.Builder
	if similar {
		b.WriteString(termx.Faint(prefix + " similar error seen " + when + " in " + where + " ("))
		b.WriteString(termx.Cyan("err show " + id))
		b.WriteString(termx.Faint(")"))
	} else {
		b.WriteString(termx.Faint(prefix + " seen " + when + " in " + where +
			" (occurrence #" + strconv.Itoa(rec.Count) + ")"))
		if rec.Solution != "" {
			b.WriteString("\n")
			b.WriteString(termx.Faint("fix: "))
			b.WriteString(termx.Bright(truncate(rec.Solution)))
			b.WriteString(termx.Faint(" ("))
			b.WriteString(termx.Cyan("err show " + id))
			b.WriteString(termx.Faint(" for details)"))
		} else {
			b.WriteString("\n")
			b.WriteString(termx.Faint("seen but no solution recorded ("))
			b.WriteString(termx.Cyan("err fix"))
			b.WriteString(termx.Faint(" to add, "))
			b.WriteString(termx.Cyan("err show " + id))
			b.WriteString(termx.Faint(")"))
		}
	}
	fmt.Fprintln(w, b.String())
}

// PrintSolved writes the "did you just fix this?" nudge after a successful
// command near a fresh pending error (dev-guide §7.2 DETECTED_SUCCESS):
// exactly two lines, "looks fixed" in bright green, signature in bright
// white, command name in cyan.
func PrintSolved(w io.Writer, e *store.Error) {
	if e == nil {
		return
	}
	fmt.Fprintln(w, termx.Faint(prefix+" ")+termx.Green("looks fixed")+termx.Faint(": ")+
		termx.Bright(truncate(e.Signature))+"\n"+termx.Cyan("err fix")+termx.Faint(" to record the solution"))
}

// truncate collapses whitespace and cuts to maxSolutionCols display
// columns without splitting a rune.
func truncate(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return termx.Truncate(s, maxSolutionCols)
}
