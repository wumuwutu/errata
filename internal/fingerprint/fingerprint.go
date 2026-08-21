package fingerprint

import "fmt"

// Fingerprint runs the full v1 pipeline on raw stderr: strip ANSI, extract
// the Python/Node error signature, and hash it. An empty signature (and
// hex) means the output is not a recognizable Python/Node error and must
// be skipped (precision-first: never guess).
func Fingerprint(stderr string) (lang, signature, hex string) {
	lang, signature = ExtractSignature(stderr)
	if signature == "" {
		return lang, "", ""
	}
	return lang, signature, fmt.Sprintf("%016x", SimHash(signature))
}
