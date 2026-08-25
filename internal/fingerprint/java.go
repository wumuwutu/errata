package fingerprint

import (
	"regexp"
	"strings"
)

var (
	// Java: Exception in thread "main" java.lang.NullPointerException: msg
	// The exception class must be dotted (java.lang.*, com.acme.*) so a
	// stray word after the marker never qualifies.
	javaExcRe = regexp.MustCompile(`^Exception in thread "[^"]*" ([A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)+)(?::\s*(.*))?$`)
	// Top stack frame: "at com.app.Main.main(Main.java:12)".
	javaFrameRe = regexp.MustCompile(`^\s*at\s+[A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)+\([^)]*\)`)
)

// javaSignature claims JVM output of the "Exception in thread" shape. The
// signature is the fully qualified exception class plus the normalized
// message template; the thread name is noise and dropped. For exceptions
// without a message (a bare NullPointerException on older JVMs) the top
// stack frame is the only identity there is, so it signs instead.
func javaSignature(text string, disabled map[string]bool) (string, bool) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		m := javaExcRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		sig := m[1] + ":" + messageTemplate(m[2], disabled)
		if m[2] == "" {
			for _, rest := range lines[i+1:] {
				if f := javaFrameRe.FindStringSubmatch(strings.TrimSpace(rest)); f != nil {
					sig += " " + NormalizeWith(f[0], disabled)
					break
				}
			}
		}
		return sig, true
	}
	return "", false
}
