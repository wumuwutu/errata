package fingerprint

import (
	"regexp"
	"strings"
)

// cCompileRe matches gcc/clang diagnostics: "main.c:12:5: error: msg",
// "lib.h:3: error: msg" (no column), or clang's "fatal error: msg". The
// source excerpt and caret line clang prints underneath carry no signature
// and are ignored. Only error severities qualify — warnings and notes are
// not errors worth remembering (precision-first, dev-guide §6.3).
var cCompileRe = regexp.MustCompile(`^[\w./\\-]+\.(?:c|cc|cpp|cxx|h|hh|hpp):\d+(?::\d+)?:\s+(?:fatal )?error:\s*(.+)$`)

// cSignature claims gcc/clang compile errors. The signature drops the
// volatile file:line:col prefix; the normalized message is the identity.
func cSignature(text string, disabled map[string]bool) (string, bool) {
	found := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := cCompileRe.FindStringSubmatch(line); m != nil {
			found = "error: " + NormalizeWith(strings.TrimSpace(m[1]), disabled)
		}
	}
	if found == "" {
		return "", false
	}
	return found, true
}
