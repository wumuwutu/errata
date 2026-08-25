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
	// Ubuntu/Debian command-not-found helper:
	// "Command 'pip' not found, but can be installed with: ..."
	// (different shape from the shell's own "bash: x: command not found")
	regexp.MustCompile(`^Command '[^']+' not found\b`),
}

// threadRe targets the one quoted-but-volatile value in the generic
// markers: the thread name in Java's "Exception in thread" line is noise,
// unlike quoted values elsewhere which carry error identity.
var threadRe = regexp.MustCompile(`(?i)\bException in thread\s+"[^"]*"`)

// shellBuiltinRe matches a shell builtin's error line by shape, in any
// locale: shell name, builtin name, message — e.g. the zh_CN
// "bash: cd: dsad: 没有那个文件或目录", which no English keyword marker
// recognizes. Precision check: prose almost never carries "word: "
// immediately after "<shell>: ".
var shellBuiltinRe = regexp.MustCompile(`^(?:bash|zsh|sh|dash|fish): [a-z][a-z0-9_-]*: .+$`)

// shellOperandRe isolates the volatile operand in that shape ("dsad"
// above): the segment after "shell: builtin:" up to the next ": " is the
// argument the user typed, not the error's identity — same role as <PATH>
// in normalization. Lines without a second colon (e.g. "bash: cd: too
// many arguments") carry no operand and stay untouched.
var shellOperandRe = regexp.MustCompile(`^((?:bash|zsh|sh|dash|fish): [a-z][a-z0-9_-]*: )[^:]+(: .+)$`)

// genericSignature is the fallback extractor: registered last, it claims
// output no language extractor recognized, but only when a line carries an
// unambiguous error marker or the shell-builtin shape. The signature is
// the last matching line, normalized; the language is reported as
// "unknown".
func genericSignature(text string, disabled map[string]bool) (string, bool) {
	found := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matched := false
		for _, re := range genericMarkers {
			if re.MatchString(line) {
				line = threadRe.ReplaceAllString(line, "Exception in thread <THREAD>")
				found = NormalizeWith(line, disabled)
				matched = true
				break
			}
		}
		// Keyword markers win first (existing signatures must not move);
		// the structural shell-builtin rule only claims what they miss —
		// that is exactly the localized-output case.
		if !matched && shellBuiltinRe.MatchString(line) {
			line = shellOperandRe.ReplaceAllString(line, "${1}<ARG>${2}")
			found = NormalizeWith(line, disabled)
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
