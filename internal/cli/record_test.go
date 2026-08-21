package cli

import (
	"testing"

	"github.com/wumuwutu/dejavu/internal/store"
)

type fakeMatcher struct {
	exact   *store.Error
	similar *store.Error
}

func (f fakeMatcher) Exact(fp string) (*store.Error, error)   { return f.exact, nil }
func (f fakeMatcher) Similar(fp string) (*store.Error, error) { return f.similar, nil }

func TestFindHitExactWins(t *testing.T) {
	e := &store.Error{ID: 1, Count: 2}
	m := fakeMatcher{exact: e, similar: &store.Error{ID: 9}}
	hit, similar := findHit(m, "fp")
	if hit != e || similar {
		t.Fatalf("hit=%+v similar=%v, want exact hit", hit, similar)
	}
	if hit.Count != 3 {
		t.Fatalf("count = %d, want 3 (this occurrence included)", hit.Count)
	}
}

func TestFindHitSimilarFallback(t *testing.T) {
	s := &store.Error{ID: 7}
	m := fakeMatcher{similar: s}
	hit, similar := findHit(m, "fp")
	if hit != s || !similar {
		t.Fatalf("hit=%+v similar=%v, want degraded similar hit", hit, similar)
	}
	if hit.Count != 0 {
		t.Fatal("similar hit must not bump the matched record's count")
	}
}

func TestFindHitNone(t *testing.T) {
	hit, similar := findHit(fakeMatcher{}, "fp")
	if hit != nil || similar {
		t.Fatalf("hit=%+v similar=%v, want none", hit, similar)
	}
}
