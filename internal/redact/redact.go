// Package redact scrubs credentials out of stderr before it is
// fingerprinted or stored (privacy red line, dev-guide §9): an error
// library that remembers passwords would be a liability. The rules are
// deliberately conservative and all live in this one file — audit here,
// extend here.
package redact

import "regexp"

// mask replaces every matched secret.
const mask = "***"

// rules run in order; every rule replaces its sensitive capture with mask.
// Kept short and specific on purpose: a broad rule that eats a legitimate
// token out of an error message would corrupt fingerprints (and error
// identity), which is worse than letting an exotic secret shape through —
// err ignore stays the tool for the exotic cases.
var rules = []struct {
	re   *regexp.Regexp
	repl string
}{
	// Credentials embedded in a URL: scheme://user:pass@host keeps scheme
	// and user, masks the password.
	{regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^:/\s"']+:)[^@/\s"']+@`), "${1}" + mask + "@"},
	// key=value / key: value forms for the usual secret names
	// (case-insensitive). The key may carry a prefix joined by _ . or -
	// (AUTH_TOKEN, db-password, …) as long as it ENDS with a secret name,
	// so "secretary:" never matches. The key stays — "password=***" is
	// still a useful signature — only the value is masked. Quoted values
	// are masked whole; an optional "Bearer" prefix goes with the token.
	{regexp.MustCompile(`(?i)\b([\w.-]*(?:password|passwd|token|secret|api_?key|access_key|authorization))(\s*[:=]\s*)(["'])[^"']*(["'])`), "${1}${2}${3}" + mask + "${4}"},
	{regexp.MustCompile(`(?i)\b([\w.-]*(?:password|passwd|token|secret|api_?key|access_key|authorization))(\s*[:=]\s*)(?:Bearer[ \t]+)?[^\s,;"']+`), "${1}${2}" + mask},
	// Well-known token prefixes: GitHub (ghp_/gho_/github_pat_), OpenAI
	// style (sk-), AWS access key id (AKIA…), Slack (xox[baprs]-).
	{regexp.MustCompile(`\b(?:ghp|gho)_[A-Za-z0-9]{20,}`), mask},
	{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), mask},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}`), mask},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), mask},
	{regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), mask},
	// JWT: three base64url segments, the first starting with eyJ (the
	// base64url of `{"`). The eyJ anchor keeps dotted prose out.
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`), mask},
}

// String returns s with recognized credentials masked.
func String(s string) string {
	for _, r := range rules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}

// Bytes is String for byte slices.
func Bytes(b []byte) []byte {
	return []byte(String(string(b)))
}
