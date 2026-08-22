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

// checkShape enforces the restraint red lines shared by all hints:
// at most two lines, every line starting at column 0, ASCII dashes only.
func checkShape(t *testing.T, out string) []string {
	t.Helper()
	if !strings.HasPrefix(out, gray) || !strings.Contains(out, reset) {
		t.Fatalf("missing gray ANSI: %q", out)
	}
	if strings.Contains(out, "──") || strings.Contains(out, "—") {
		t.Fatalf("hint must use ASCII '-' only: %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 2 {
		t.Fatalf("hint has %d lines, max 2: %q", len(lines), out)
	}
	for _, ln := range lines {
		ln = strings.TrimPrefix(ln, gray)
		ln = strings.TrimPrefix(ln, cyan)
		ln = strings.TrimSuffix(ln, reset)
		if ln != strings.TrimLeft(ln, " \t") {
			t.Errorf("hint line not left-aligned: %q", ln)
		}
	}
	return lines
}

func TestPrintExactHitWithSolution(t *testing.T) {
	r := rec()
	r.Solution = "reinstall with conda deactivated"
	var b bytes.Buffer
	Print(&b, r, false)
	out := b.String()
	checkShape(t, out)

	for _, want := range []string{"- err - seen 2025-11-03", "projects/api", "occurrence #3", "fix: reinstall", "err show 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("hint missing %q: %q", want, out)
		}
	}
	// Command names stand out in cyan inside the gray hint.
	if !strings.Contains(out, cyan+"err show 3"+gray) {
		t.Errorf("err show 3 not cyan-highlighted: %q", out)
	}
}

func TestPrintExactHitNoSolution(t *testing.T) {
	var b bytes.Buffer
	Print(&b, rec(), false)
	out := b.String()
	checkShape(t, out)
	if !strings.Contains(out, "no solution recorded") {
		t.Fatalf("unresolved hint wrong: %q", out)
	}
	for _, cmd := range []string{cyan + "err fix" + gray, cyan + "err show 3" + gray} {
		if !strings.Contains(out, cmd) {
			t.Errorf("missing cyan command %q: %q", cmd, out)
		}
	}
}

func TestPrintSimilarIsOneLine(t *testing.T) {
	var b bytes.Buffer
	Print(&b, rec(), true)
	out := b.String()
	lines := checkShape(t, out)
	if len(lines) != 1 {
		t.Fatalf("similar hint should be 1 line, got %d: %q", len(lines), out)
	}
	if !strings.Contains(out, "similar error") {
		t.Fatalf("missing similar marker: %q", out)
	}
}

func TestPrintSolvedTwoLines(t *testing.T) {
	r := rec()
	r.Signature = "TypeError: boom"
	var b bytes.Buffer
	PrintSolved(&b, r)
	out := b.String()
	lines := checkShape(t, out)
	if len(lines) != 2 {
		t.Fatalf("solved hint should be 2 lines, got %d: %q", len(lines), out)
	}
	if !strings.Contains(lines[0], "- err - looks fixed: TypeError: boom") {
		t.Fatalf("first line wrong: %q", lines[0])
	}
	plain := strings.NewReplacer(gray, "", cyan, "", reset, "").Replace(lines[1])
	if plain != "err fix to record the solution" {
		t.Fatalf("second line wrong: %q", plain)
	}
	if !strings.Contains(lines[1], cyan+"err fix"+gray) {
		t.Fatalf("err fix not cyan-highlighted: %q", lines[1])
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
