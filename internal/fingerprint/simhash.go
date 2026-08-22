package fingerprint

import (
	"hash/fnv"
	"regexp"
	"strings"
)

// SimilarityThreshold is the maximum Hamming distance at which two
// fingerprints are presented as "similar errors" (dev-guide §6.2, initial
// value 6, precision-first).
const SimilarityThreshold = 6

// Tokens are lowercase word runs. CJK ideographs count too: without them
// a localized message (zh_CN "bash: cd: x: 没有那个文件或目录" vs
// "权限不够") contributes zero tokens and distinct errors would share a
// fingerprint — a precision violation (dev-guide §6.3).
var tokenRe = regexp.MustCompile(`[a-z0-9_<>\p{Han}]+`)

// SimHash computes a 64-bit SimHash of text (a normalized error
// signature). Deterministic: identical signatures always yield identical
// fingerprints, and small wording differences flip only a few bits.
func SimHash(text string) uint64 {
	var v [64]int
	for _, tok := range tokenRe.FindAllString(strings.ToLower(text), -1) {
		h := fnv.New64a()
		h.Write([]byte(tok))
		bits := h.Sum64()
		for i := 0; i < 64; i++ {
			if bits&(1<<uint(i)) != 0 {
				v[i]++
			} else {
				v[i]--
			}
		}
	}
	var out uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			out |= 1 << uint(i)
		}
	}
	return out
}

// HammingDistance counts the differing bits between two fingerprints.
func HammingDistance(a, b uint64) int {
	x := a ^ b
	n := 0
	for x != 0 {
		n += int(x & 1)
		x >>= 1
	}
	return n
}
