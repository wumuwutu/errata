// Package match defines how a fresh fingerprint finds prior records:
// an exact hit, or a degraded "similar" match. SimHash is the only
// implementation today; the embedding fallback of dev-guide §6.2 will
// slot in as another Matcher without touching the recording path.
package match

import (
	"github.com/wumuwutu/errata/internal/fingerprint"
	"github.com/wumuwutu/errata/internal/store"
)

// Matcher finds a prior record for a fingerprint.
type Matcher interface {
	// Exact returns the record with exactly this fingerprint, or nil.
	Exact(fp string) (*store.Error, error)
	// Similar returns the closest record within the implementation's
	// similarity threshold, or nil when nothing is close enough.
	Similar(fp string) (*store.Error, error)
}

// SimHash matches on the 64-bit SimHash fingerprint: identical hash is an
// exact hit; Hamming distance within fingerprint.SimilarityThreshold is a
// similar error.
type SimHash struct {
	Store *store.Store
}

// Exact implements Matcher.
func (m SimHash) Exact(fp string) (*store.Error, error) {
	return m.Store.FindByFingerprint(fp)
}

// Similar implements Matcher.
func (m SimHash) Similar(fp string) (*store.Error, error) {
	e, _, err := m.Store.FindSimilar(fp, fingerprint.SimilarityThreshold)
	return e, err
}
