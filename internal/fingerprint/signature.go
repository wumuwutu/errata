package fingerprint

import (
	"regexp"
	"strings"
)

// Languages errata fingerprints in v1 (dev-guide §6.5: MVP restraint).
const (
	LangPython  = "python"
	LangNode    = "node"
	LangUnknown = "unknown"
)

var (
	// Python, traceback present: the final exception line, e.g.
	// "TypeError: unsupported ..." or dotted
	// "django.core.exceptions.ImproperlyConfigured: ...".
	pyExcRe = regexp.MustCompile(`^([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*):\s*(.*)$`)
	// Python, no traceback (SyntaxError family): only names carrying a
	// known exception suffix qualify, so prose like "Note: ..." never
	// matches (precision-first, dev-guide §6.3).
	pyTypedExcRe = regexp.MustCompile(`^([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*(?:Error|Exception|Warning|Interrupt|Exit|Iteration)):\s*(.*)$`)
	// Bare exception with no message, e.g. "KeyboardInterrupt".
	pyBareExcRe = regexp.MustCompile(`^([A-Za-z_]\w*(?:Error|Exception|Warning|Interrupt|Exit|Iteration))$`)
	// SyntaxError-family proof: the 'File "...", line N' line that always
	// accompanies the block. (The caret line is NOT proof — Node prints
	// carets under source excerpts too.) Without it, a bare
	// "SomeError: msg" line is not enough — other tools print those.
	pyFileLineRe = regexp.MustCompile(`(?m)^\s*File ".*", line \d+`)

	// Node: "TypeError: boom" or bare "Error: boom", confirmed by stack
	// frames ("    at ...") so random prose never qualifies.
	nodeErrRe  = regexp.MustCompile(`^((?:[A-Za-z_$][\w$]*)(?:Error|Exception)):\s*(.*)$`)
	nodeBareRe = regexp.MustCompile(`^Error:\s*(.+)$`)
	nodeFrame  = regexp.MustCompile(`(?m)^\s+at\s+\S`)
)

// Extractor pulls a normalized error signature out of ANSI-stripped
// stderr text. ok=false means "not my language". Implementations must be
// conservative — precision over recall (dev-guide §6.3): when unsure,
// return ok=false and let the error go unrecorded.
type Extractor func(text string, disabled map[string]bool) (signature string, ok bool)

// registry holds the language extractors in probe order. Python comes
// first: its traceback marker is unambiguous, while Node's stack frames
// could appear in other tools' output.
var registry = []struct {
	lang string
	ex   Extractor
}{
	{LangPython, pythonSignature},
	{LangNode, nodeSignature},
}

// Register adds a language extractor. Supporting a new language is meant
// to be one file (e.g. java.go) plus one Register call in its init().
func Register(lang string, ex Extractor) {
	registry = append(registry, struct {
		lang string
		ex   Extractor
	}{lang, ex})
}

// ExtractSignature pulls a normalized error signature out of raw stderr,
// using the production rule set (DefaultDisabledRules). An empty signature
// means "no recognizable error marker" and the caller must skip it.
func ExtractSignature(stderr string) (lang, signature string) {
	return ExtractSignatureWith(stderr, DefaultDisabledRules)
}

// ExtractSignatureWith is ExtractSignature with selected normalization
// rules disabled (ablation runs in the eval toolchain).
func ExtractSignatureWith(stderr string, disabled map[string]bool) (lang, signature string) {
	text := StripANSI(stderr)
	for _, r := range registry {
		if sig, ok := r.ex(text, disabled); ok {
			return r.lang, sig
		}
	}
	return LangUnknown, ""
}

// pythonSignature has two paths:
//   - traceback present: take the LAST "Name: message" line (the raised
//     exception of a chained traceback is last); the marker makes any
//     identifier-shaped name safe to accept.
//   - no traceback: only the SyntaxError family block qualifies, proven
//     by its 'File "...", line N' line, and the exception name must carry
//     a known suffix (pyTypedExcRe). Both constraints together keep Node
//     errors and prose out (precision-first, §6.3).
func pythonSignature(text string, disabled map[string]bool) (string, bool) {
	if strings.Contains(text, "Traceback (most recent call last):") {
		return scanPythonException(text, pyExcRe, disabled)
	}
	if !pyFileLineRe.MatchString(text) {
		return "", false
	}
	return scanPythonException(text, pyTypedExcRe, disabled)
}

// scanPythonException returns the last matching exception line as
// "Name: <normalized message>", or ok=false when nothing qualifies.
func scanPythonException(text string, re *regexp.Regexp, disabled map[string]bool) (string, bool) {
	found := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := re.FindStringSubmatch(line); m != nil {
			found = m[1] + ":" + messageTemplate(m[2], disabled)
		} else if m := pyBareExcRe.FindStringSubmatch(line); m != nil {
			found = m[1] + ":"
		}
	}
	if found == "" {
		return "", false
	}
	return found, true
}

// nodeSignature requires at least one "at ..." stack frame, then takes the
// first error line (Node prints the error before its stack).
func nodeSignature(text string, disabled map[string]bool) (string, bool) {
	if !nodeFrame.MatchString(text) {
		return "", false
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := nodeErrRe.FindStringSubmatch(line); m != nil {
			return m[1] + ":" + messageTemplate(m[2], disabled), true
		}
		if m := nodeBareRe.FindStringSubmatch(line); m != nil {
			return "Error:" + messageTemplate(m[1], disabled), true
		}
	}
	return "", false
}

// messageTemplate normalizes a raw exception message into a stable
// template, keeping at most one line.
func messageTemplate(msg string, disabled map[string]bool) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	return " " + NormalizeWith(msg, disabled)
}
