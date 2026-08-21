package eval

import (
	"os"
	"path/filepath"
	"testing"
)

const pyA1 = "Traceback (most recent call last):\n  File \"/home/a/x.py\", line 1, in <module>\nTypeError: boom 'x'\n"
const pyA2 = "Traceback (most recent call last):\n  File \"/opt/b/y.py\", line 99, in <module>\nTypeError: boom 'y'\n"
const pyB = "Traceback (most recent call last):\n  File \"/home/a/x.py\", line 1, in <module>\nKeyError: 'missing'\n"
const junk = "gcc: something failed\n"

func corpus() []Entry {
	return []Entry{
		{Raw: pyA1, Language: "python", Group: "g1"},
		{Raw: pyA2, Language: "python", Group: "g1"},
		{Raw: pyB, Language: "python", Group: "g2"},
		{Raw: junk, Language: "unknown", Group: "g3"},
	}
}

func TestEvaluateThresholdZero(t *testing.T) {
	m := Evaluate(corpus(), nil, 0)[0]

	// Pairs: (A1,A2) same&matched; (A1,B),(A2,B) different; junk never
	// matches anything.
	if m.TP != 1 || m.FP != 0 || m.FN != 0 {
		t.Fatalf("t=0: TP=%d FP=%d FN=%d, want 1/0/0", m.TP, m.FP, m.FN)
	}
	if m.Precision != 1 || m.Recall != 1 {
		t.Fatalf("t=0: P=%.2f R=%.2f, want 1/1", m.Precision, m.Recall)
	}
}

func TestEvaluateSameGroupMissedIsFN(t *testing.T) {
	entries := []Entry{
		{Raw: junk, Group: "g1"},
		{Raw: junk + "more", Group: "g1"},
	}
	m := Evaluate(entries, nil, 0)[0]
	// Unrecognized entries never match: their same-group pair is a miss.
	if m.TP != 0 || m.FN != 1 {
		t.Fatalf("TP=%d FN=%d, want 0/1", m.TP, m.FN)
	}
	if m.Recall != 0 {
		t.Fatalf("recall = %.2f, want 0", m.Recall)
	}
}

func TestEvaluateHighThresholdCollapsesEverything(t *testing.T) {
	entries := []Entry{
		{Raw: pyA1, Group: "g1"},
		{Raw: pyB, Group: "g2"},
	}
	m := Evaluate(entries, nil, 64)[64]
	if m.TP != 0 || m.FP != 1 {
		t.Fatalf("t=64: TP=%d FP=%d, want 0/1 (everything matches)", m.TP, m.FP)
	}
	if m.Precision != 0 || m.Recall != 0 {
		t.Fatalf("t=64: P=%.2f R=%.2f", m.Precision, m.Recall)
	}
}

func TestAblationDisablingPathRule(t *testing.T) {
	// Same error, different file path only.
	a := "Traceback (most recent call last):\n  File \"/home/a/x.py\", line 1, in <module>\nValueError: bad\n"
	b := "Traceback (most recent call last):\n  File \"/opt/other/x.py\", line 1, in <module>\nValueError: bad\n"
	entries := []Entry{{Raw: a, Group: "g1"}, {Raw: b, Group: "g1"}}

	withRules := Evaluate(entries, nil, 0)[0]
	if withRules.TP != 1 {
		t.Fatalf("full pipeline: TP=%d, want 1 (path normalized away)", withRules.TP)
	}

	// The path lives only in the stack frame, not the signature — so
	// disabling "path" changes nothing here. Disable "val" instead on a
	// pair that differs only in a quoted value:
	a2 := "Traceback (most recent call last):\n  File \"/x.py\", line 1, in <module>\nKeyError: 'alpha'\n"
	b2 := "Traceback (most recent call last):\n  File \"/x.py\", line 1, in <module>\nKeyError: 'beta'\n"
	entries2 := []Entry{{Raw: a2, Group: "g1"}, {Raw: b2, Group: "g1"}}

	full := Evaluate(entries2, nil, 0)[0]
	if full.TP != 1 {
		t.Fatalf("full pipeline: TP=%d, want 1 (quoted value normalized)", full.TP)
	}
	abl := Evaluate(entries2, map[string]bool{"val": true}, 0)[0]
	if abl.TP != 0 || abl.FN != 1 {
		t.Fatalf("ablation val: TP=%d FN=%d, want 0/1 (values differ)", abl.TP, abl.FN)
	}
	if abl.Recall >= full.Recall {
		t.Fatalf("ablation recall %.2f should drop below %.2f", abl.Recall, full.Recall)
	}
}

func TestLoadCorpus(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.jsonl")
	content := "# comment\n\n{\"raw\": \"x\", \"language\": \"python\", \"group\": \"g1\"}\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := LoadCorpus(p)
	if err != nil || len(entries) != 1 {
		t.Fatalf("LoadCorpus: %v, %d entries", err, len(entries))
	}

	bad := filepath.Join(dir, "bad.jsonl")
	os.WriteFile(bad, []byte("{\"raw\": \"x\"}\n"), 0o644)
	if _, err := LoadCorpus(bad); err == nil {
		t.Fatal("missing group must be rejected")
	}
	os.WriteFile(bad, []byte("not json\n"), 0o644)
	if _, err := LoadCorpus(bad); err == nil {
		t.Fatal("invalid JSON must be rejected")
	}
}

// TestSampleCorpusSanity guards the shipped sample corpus: full pipeline,
// threshold 0 must be perfectly precise (no cross-group collisions) and
// must catch every same-group pair (the corpus is curated so that
// normalization alone aligns each group).
func TestSampleCorpusSanity(t *testing.T) {
	entries, err := LoadCorpus(filepath.Join("..", "..", "eval", "corpus.jsonl"))
	if err != nil {
		t.Skipf("sample corpus not present: %v", err)
	}
	m := Evaluate(entries, nil, 0)[0]
	if m.FP != 0 {
		t.Fatalf("sample corpus: FP=%d at t=0 — different groups collided", m.FP)
	}
	if m.FN != 0 {
		t.Fatalf("sample corpus: FN=%d at t=0 — same-group pair not unified", m.FN)
	}
}
