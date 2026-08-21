package fingerprint

import "fmt"

// Fingerprint runs the full v1 pipeline on raw stderr: strip ANSI, extract
// the error signature with the production rule set (quoted values kept,
// see DefaultDisabledRules), and hash it. An empty signature (and hex)
// means the output carries no recognizable error marker and must be
// skipped (precision-first: never guess).
func Fingerprint(stderr string) (lang, signature, hex string) {
	return FingerprintWith(stderr, DefaultDisabledRules)
}

// FingerprintWith is Fingerprint with selected normalization rules
// disabled (ablation runs in the eval toolchain).
func FingerprintWith(stderr string, disabled map[string]bool) (lang, signature, hex string) {
	lang, signature = ExtractSignatureWith(stderr, disabled)
	if signature == "" {
		return lang, "", ""
	}
	return lang, signature, fmt.Sprintf("%016x", SimHash(signature))
}
