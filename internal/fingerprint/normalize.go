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

// RuleNames lists the normalization rules, in pipeline order. Used by the
// eval toolchain for ablation runs (dev-guide §6.4).
var RuleNames = []string{"uuid", "ts", "ip", "addr", "path", "val", "num"}

// DefaultDisabledRules are rules the production pipeline leaves off.
// "val" is off: the quoted value IS the error's identity — with it
// blanked, "No module named 'numpy'" and "'torch'" collapse into one
// fingerprint and the wrong fix would be served (precision red line,
// dev-guide §6.3/§9). The rule code stays for ablation runs.
var DefaultDisabledRules = map[string]bool{"val": true}

// Normalize rewrites volatile fragments of s into stable placeholders:
//
//	UUID → <UUID>, timestamps → <TS>, IP addresses → <IP>,
//	hex addresses → <ADDR>, absolute paths → <PATH>,
//	remaining numbers → <N>
//
// Quoted values are kept (see DefaultDisabledRules).
func Normalize(s string) string {
	return NormalizeWith(s, DefaultDisabledRules)
}

// NormalizeWith applies the full rule set minus the given rules
// (ablation). Pass DefaultDisabledRules for production behavior, nil for
// everything on.
func NormalizeWith(s string, disabled map[string]bool) string {
	off := func(rule string) bool { return disabled[rule] }
	if !off("uuid") {
		s = uuidRe.ReplaceAllString(s, "<UUID>")
	}
	if !off("ts") {
		s = tsRe.ReplaceAllString(s, "<TS>")
	}
	if !off("ip") {
		s = ipRe.ReplaceAllString(s, "<IP>")
	}
	if !off("addr") {
		s = addrRe.ReplaceAllString(s, "<ADDR>")
	}
	if !off("path") {
		s = winPath.ReplaceAllString(s, "<PATH>")
		s = unixPathRe.ReplaceAllString(s, "${1}<PATH>")
	}
	if !off("val") {
		s = squoteRe.ReplaceAllString(s, "<VAL>")
		s = dquoteRe.ReplaceAllString(s, "<VAL>")
		s = bquoteRe.ReplaceAllString(s, "<VAL>")
	}
	if !off("num") {
		s = numberRe.ReplaceAllString(s, "<N>")
	}
	return s
}
