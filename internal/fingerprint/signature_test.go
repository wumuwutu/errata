package fingerprint

import "testing"

// TestRegistryOrderLocked pins the probe order: precise extractors in a
// fixed sequence, the generic fallback strictly after all of them — its
// markers would otherwise claim Java/gcc/Go output first. Relative order is
// asserted rather than equality because tests may Register extra
// extractors.
func TestRegistryOrderLocked(t *testing.T) {
	want := []string{LangPython, LangNode, LangJava, LangGo, LangC, LangUnknown}
	prev := -1
	for _, lang := range want {
		idx := -1
		for i, r := range registry {
			if r.lang == lang {
				idx = i
				break
			}
		}
		if idx < 0 {
			t.Fatalf("%s extractor not registered", lang)
		}
		if idx <= prev {
			t.Fatalf("registry order broken: %s at %d, previous at %d", lang, idx, prev)
		}
		prev = idx
	}
	// No built-in may sit behind the fallback, whatever tests appended.
	fallback := -1
	for i, r := range registry {
		if r.lang == LangUnknown {
			fallback = i
		}
	}
	for i, r := range registry[:fallback] {
		if r.lang == LangUnknown && i != fallback {
			t.Fatalf("generic fallback registered twice (at %d and %d)", i, fallback)
		}
	}
}
