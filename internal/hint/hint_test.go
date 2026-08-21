package hint

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/wumuwutu/dejavu/internal/store"
)

func rec() *store.Error {
	return &store.Error{
		ID:         3,
		ProjectDir: "/nonexistent-dir-x/projects/api",
		FirstSeen:  time.Date(2025, 11, 3, 0, 0, 0, 0, time.UTC),
		Count:      3,
	}
}

func TestPrintExactHitWithSolution(t *testing.T) {
	r := rec()
	r.Solution = "reinstall with conda deactivated"
	var b bytes.Buffer
	Print(&b, r, false)
	out := b.String()

	if !strings.HasPrefix(out, gray) || !strings.Contains(out, reset) {
		t.Fatalf("missing gray ANSI: %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 2 {
		t.Fatalf("hint has %d lines, max 2: %q", len(lines), out)
	}
	// Every line starts at column 0 (modulo the gray escape).
	for _, ln := range lines {
		ln = strings.TrimPrefix(ln, gray)
		ln = strings.TrimSuffix(ln, reset)
		if ln != strings.TrimLeft(ln, " \t") {
			t.Errorf("hint line not left-aligned: %q", ln)
		}
	}
	for _, want := range []string{"2025-11-03", "projects/api", "occurrence #3", "fix: reinstall", "err show 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("hint missing %q: %q", want, out)
		}
	}
}

func TestPrintExactHitNoSolution(t *testing.T) {
	var b bytes.Buffer
	Print(&b, rec(), false)
	out := b.String()
	if !strings.Contains(out, "no solution recorded") || !strings.Contains(out, "err fix") {
		t.Fatalf("unresolved hint wrong: %q", out)
	}
}

func TestPrintSimilarIsOneLine(t *testing.T) {
	var b bytes.Buffer
	Print(&b, rec(), true)
	out := b.String()
	lines := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if lines != 1 {
		t.Fatalf("similar hint should be 1 line, got %d: %q", lines, out)
	}
	if !strings.Contains(out, "similar error") {
		t.Fatalf("missing similar marker: %q", out)
	}
}

func TestPrintSolvedLeftAligned(t *testing.T) {
	r := rec()
	r.Signature = "TypeError: boom"
	var b bytes.Buffer
	PrintSolved(&b, r)
	out := b.String()
	ln := strings.TrimPrefix(strings.TrimRight(out, "\n"), gray)
	if ln != strings.TrimLeft(ln, " \t") {
		t.Fatalf("solved hint not left-aligned: %q", out)
	}
	if !strings.Contains(out, "looks fixed") || !strings.Contains(out, "err fix") {
		t.Fatalf("solved hint wrong: %q", out)
	}
}

func TestTruncateLongSolution(t *testing.T) {
	long := strings.Repeat("x", 200)
	if got := truncate(long); len([]rune(got)) > maxSolutionRunes {
		t.Fatalf("truncate gave %d runes", len([]rune(got)))
	}
	if got := truncate("a\nb\tc"); got != "a b c" {
		t.Fatalf("whitespace collapse: %q", got)
	}
}
