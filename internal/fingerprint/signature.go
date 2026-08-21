package fingerprint

import (
	"regexp"
	"strings"
)

// Languages dejavu fingerprints in v1 (dev-guide §6.5: MVP restraint).
const (
	LangPython  = "python"
	LangNode    = "node"
	LangUnknown = "unknown"
)

var (
	// Python: the final exception line, e.g. "TypeError: unsupported ..."
	// or dotted "django.core.exceptions.ImproperlyConfigured: ...".
	pyExcRe = regexp.MustCompile(`^([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*):\s*(.*)$`)
	// Bare exception with no message, e.g. "KeyboardInterrupt".
	pyBareExcRe = regexp.MustCompile(`^([A-Za-z_]\w*(?:Error|Exception|Warning|Interrupt|Exit|Iteration))$`)

	// Node: "TypeError: boom" or bare "Error: boom", confirmed by stack
	// frames ("    at ...") so random prose never qualifies.
	nodeErrRe  = regexp.MustCompile(`^((?:[A-Za-z_$][\w$]*)(?:Error|Exception)):\s*(.*)$`)
	nodeBareRe = regexp.MustCompile(`^Error:\s*(.+)$`)
	nodeFrame  = regexp.MustCompile(`(?m)^\s+at\s+\S`)
)

// ExtractSignature pulls a normalized error signature out of raw stderr.
// It returns the detected language and the signature; an empty signature
// means "not a recognizable Python/Node error" and the caller must skip it.
func ExtractSignature(stderr string) (lang, signature string) {
	return ExtractSignatureWith(stderr, nil)
}

// ExtractSignatureWith is ExtractSignature with selected normalization
// rules disabled (ablation runs in the eval toolchain).
func ExtractSignatureWith(stderr string, disabled map[string]bool) (lang, signature string) {
	text := StripANSI(stderr)
	if sig, ok := pythonSignature(text, disabled); ok {
		return LangPython, sig
	}
	if sig, ok := nodeSignature(text, disabled); ok {
		return LangNode, sig
	}
	return LangUnknown, ""
}

// pythonSignature requires the traceback marker, then takes the LAST
// exception line (the raised exception of a chained traceback is last).
func pythonSignature(text string, disabled map[string]bool) (string, bool) {
	if !strings.Contains(text, "Traceback (most recent call last):") {
		return "", false
	}
	found := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := pyExcRe.FindStringSubmatch(line); m != nil {
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
