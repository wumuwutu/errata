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
	r.Solution = "conda deactivate 后重装"
	var b bytes.Buffer
	Print(&b, r, false)
	out := b.String()

	if !strings.HasPrefix(out, gray) || !strings.Contains(out, reset) {
		t.Fatalf("missing gray ANSI: %q", out)
	}
	lines := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if lines > 2 {
		t.Fatalf("hint has %d lines, max 2: %q", lines, out)
	}
	for _, want := range []string{"2025-11-03", "projects/api", "第3次", "解法：conda", "err show 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("hint missing %q: %q", want, out)
		}
	}
}

func TestPrintExactHitNoSolution(t *testing.T) {
	var b bytes.Buffer
	Print(&b, rec(), false)
	out := b.String()
	if !strings.Contains(out, "还没记录解法") || !strings.Contains(out, "err fix") {
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
	if !strings.Contains(out, "相似错误") {
		t.Fatalf("missing similar marker: %q", out)
	}
}

func TestTruncateLongSolution(t *testing.T) {
	long := strings.Repeat("解", 200)
	if got := truncate(long); len([]rune(got)) > maxSolutionRunes {
		t.Fatalf("truncate gave %d runes", len([]rune(got)))
	}
	if got := truncate("a\nb\tc"); got != "a b c" {
		t.Fatalf("whitespace collapse: %q", got)
	}
}
