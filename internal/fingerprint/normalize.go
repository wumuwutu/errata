// Package fingerprint turns raw stderr into a stable error signature and a
// 64-bit SimHash fingerprint: same error with different paths/line
// numbers/PIDs/timestamps must hash identically; different errors must not.
//
// Precision beats recall (dev-guide §6.3): anything we cannot confidently
// attribute to a supported language (Python/Node) yields an empty signature
// and is skipped rather than guessed.
package fingerprint

import (
	"regexp"
	"strings"
)

var (
	ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")

	// Order matters: more specific patterns run before the catch-all
	// number rule so their digits survive.
	uuidRe  = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	tsRe    = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?)?\b|\b\d{2}:\d{2}:\d{2}(\.\d+)?\b`)
	ipRe    = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	addrRe  = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b`)
	winPath = regexp.MustCompile(`\b[A-Za-z]:\\(?:[^\\/:*?"<>|\s]+\\)*[^\\/:*?"<>|\s]*`)
	// Unix absolute path: a "/" preceded by start/whitespace/opening
	// punctuation, followed by at least one path segment.
	unixPathRe = regexp.MustCompile(`(^|[\s(:=,"'<{\[])/(?:[\w.~+\-]+/)*[\w.~+\-]+`)
	squoteRe   = regexp.MustCompile(`'[^']*'`)
	dquoteRe   = regexp.MustCompile(`"[^"]*"`)
	bquoteRe   = regexp.MustCompile("`[^`]*`")
	numberRe   = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
)

// StripANSI removes ANSI escape sequences (colors, cursor moves, OSC
// hyperlinks) and carriage returns from terminal output.
func StripANSI(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	return strings.ReplaceAll(s, "\r", "")
}

// Normalize rewrites volatile fragments of s into stable placeholders:
//
//	UUID → <UUID>, timestamps → <TS>, IP addresses → <IP>,
//	hex addresses → <ADDR>, absolute paths → <PATH>,
//	quoted values → <VAL>, remaining numbers → <N>
func Normalize(s string) string {
	s = uuidRe.ReplaceAllString(s, "<UUID>")
	s = tsRe.ReplaceAllString(s, "<TS>")
	s = ipRe.ReplaceAllString(s, "<IP>")
	s = addrRe.ReplaceAllString(s, "<ADDR>")
	s = winPath.ReplaceAllString(s, "<PATH>")
	s = unixPathRe.ReplaceAllString(s, "${1}<PATH>")
	s = squoteRe.ReplaceAllString(s, "<VAL>")
	s = dquoteRe.ReplaceAllString(s, "<VAL>")
	s = bquoteRe.ReplaceAllString(s, "<VAL>")
	s = numberRe.ReplaceAllString(s, "<N>")
	return s
}
