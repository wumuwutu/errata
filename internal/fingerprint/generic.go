package fingerprint

import (
	"regexp"
	"strings"
)

// genericMarkers are unambiguous error markers across languages and
// tools. The list is deliberately short and conservative (precision-first,
// dev-guide §6.3): output matching none of them is not recorded at all.
var genericMarkers = []*regexp.Regexp{
	// Java: Exception in thread "main" java.lang.NullPointerException: ...
	regexp.MustCompile(`(?i)\bException in thread\b`),
	// Go: panic: runtime error: ...
	regexp.MustCompile(`^panic:`),
	// git/curl/...: fatal: not a git repository ...
	regexp.MustCompile(`^(?:fatal|FATAL):\s`),
	// gcc/rustc: "main.c:12:5: error: ...", "gcc: error: ...", or a
	// line-start "error:" / "error[E0308]:"
	regexp.MustCompile(`^(?:[\w./\-]+:\d+(?::\d+)?:\s+|[\w./\-]+:\s+)?error(?:\[[^\]]*\])?:\s`),
	// Uncaught "SomeError: ..." / "java.lang.SomeException: ..." without
	// the "Exception in thread" prefix.
	regexp.MustCompile(`^[\w.$]*(?:Error|Exception):\s`),
	// shell / POSIX classics
	regexp.MustCompile(`\bcommand not found\b`),
	regexp.MustCompile(`\bNo such file or directory\b`),
	regexp.MustCompile(`\bPermission denied\b`),
}

// genericSignature is the fallback extractor: registered last, it claims
// output no language extractor recognized, but only when a line carries an
// unambiguous error marker. The signature is the last matching line,
// normalized; the language is reported as "unknown".
func genericSignature(text string, disabled map[string]bool) (string, bool) {
	found := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, re := range genericMarkers {
			if re.MatchString(line) {
				found = NormalizeWith(line, disabled)
				break
			}
		}
	}
	if found == "" {
		return "", false
	}
	return found, true
}

// Registered last: language-specific extractors claim their own first.
func init() {
	Register(LangUnknown, genericSignature)
}
