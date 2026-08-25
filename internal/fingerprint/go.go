package fingerprint

import (
	"regexp"
	"strings"
)

var (
	// Go runtime panic: "panic: runtime error: index out of range ..."
	// (usually followed by the goroutine stack, which carries no identity).
	goPanicRe = regexp.MustCompile(`^panic:\s*(.*)$`)
	// Go compiler: "file.go:12:3: syntax error: ..." — the .go extension
	// plus a line:col pair keep gcc/clang output from ever qualifying.
	goCompileRe = regexp.MustCompile(`^[\w./\\-]+\.go:\d+:\d+:\s+(.+)$`)
)

// goSignature claims Go runtime panics and compile-time errors. A panic
// signs as the panic line itself (stable across goroutine dumps); a compile
// error signs as its message with file and line:col dropped, so drift in
// either keeps the fingerprint stable.
func goSignature(text string, disabled map[string]bool) (string, bool) {
	found := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := goPanicRe.FindStringSubmatch(line); m != nil {
			found = "panic:" + messageTemplate(m[1], disabled)
			continue
		}
		if m := goCompileRe.FindStringSubmatch(line); m != nil {
			msg := strings.TrimSpace(m[1])
			if strings.HasPrefix(msg, "note:") || strings.HasPrefix(msg, "warning:") {
				continue // diagnostics, not errors
			}
			found = NormalizeWith(msg, disabled)
		}
	}
	if found == "" {
		return "", false
	}
	return found, true
}
