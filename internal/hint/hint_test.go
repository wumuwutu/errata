package hint

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/wumuwutu/dejavu/internal/store"
	"github.com/wumuwutu/dejavu/internal/termx"
)

func rec() *store.Error {
	return &store.Error{
		ID:         3,
		ProjectDir: "/nonexistent-dir-x/projects/api",
		FirstSeen:  time.Date(2025, 11, 3, 0, 0, 0, 0, time.UTC),
		Count:      3,
	}
}

// checkShape enforces the restraint red lines shared by all hints: at most
// two lines, every line starting at column 0, the --err-- prefix, ASCII
// dashes only.
func checkShape(t *testing.T, out string) []string {
	t.Helper()
	if strings.Contains(out, "──") || strings.Contains(out, "—") {
		t.Fatalf("hint must use ASCII dashes only: %q", out)
	}
	if !strings.Contains(out, "--err--") {
		t.Fatalf("hint missing --err-- prefix: %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 2 {
		t.Fatalf("hint has %d lines, max 2: %q", len(lines), out)
	}
	for _, ln := range lines {
		if ln != strings.TrimLeft(ln, " \t") {
			t.Errorf("hint line not left-aligned: %q", ln)
		}
	}
	return lines
}

func withNoColor(t *testing.T) {
	t.Helper()
	old := termx.NoColor
	termx.NoColor = true
	t.Cleanup(func() { termx.NoColor = old })
}

// withColor forces colors on regardless of the ambient NO_COLOR env, so
// palette assertions are deterministic on any machine.
func withColor(t *testing.T) {
	t.Helper()
	old := termx.NoColor
	termx.NoColor = false
	t.Cleanup(func() { termx.NoColor = old })
}

func TestPrintExactHitWithSolution(t *testing.T) {
	r := rec()
	r.Solution = "reinstall with conda deactivated"

	var b bytes.Buffer
	Print(&b, r, false)
	checkShape(t, b.String())

	withNoColor(t)
	b.Reset()
	Print(&b, r, false)
	out := b.String()
	for _, want := range []string{"--err-- seen 2025-11-03", "projects/api", "occurrence #3", "fix: reinstall", "err show 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("hint missing %q: %q", want, out)
		}
	}
	if strings.ContainsRune(out, 0x1b) {
		t.Fatalf("NO_COLOR mode must emit no ANSI: %q", out)
	}
}

func TestPrintColors(t *testing.T) {
	withColor(t)
	r := rec()
	r.Solution = "reinstall with conda deactivated"
	var b bytes.Buffer
	Print(&b, r, false)
	out := b.String()
	if !strings.Contains(out, "\x1b[90m--err--") {
		t.Errorf("base text not faint: %q", out)
	}
	if !strings.Contains(out, "\x1b[97mreinstall with conda deactivated") {
		t.Errorf("solution not bright: %q", out)
	}
	if !strings.Contains(out, "\x1b[36merr show 3") {
		t.Errorf("command not cyan: %q", out)
	}
}

func TestPrintExactHitNoSolution(t *testing.T) {
	withNoColor(t)
	var b bytes.Buffer
	Print(&b, rec(), false)
	out := b.String()
	checkShape(t, out)
	if !strings.Contains(out, "no solution recorded") ||
		!strings.Contains(out, "err fix") || !strings.Contains(out, "err show 3") {
		t.Fatalf("unresolved hint wrong: %q", out)
	}
}

func TestPrintSimilarIsOneLine(t *testing.T) {
	withNoColor(t)
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

	withNoColor(t)
	var b bytes.Buffer
	PrintSolved(&b, r)
	out := b.String()
	lines := checkShape(t, out)
	if len(lines) != 2 {
		t.Fatalf("solved hint should be 2 lines, got %d: %q", len(lines), out)
	}
	if lines[0] != "--err-- looks fixed: TypeError: boom" {
		t.Fatalf("first line wrong: %q", lines[0])
	}
	if lines[1] != "err fix to record the solution" {
		t.Fatalf("second line wrong: %q", lines[1])
	}
}

func TestPrintSolvedColors(t *testing.T) {
	withColor(t)
	r := rec()
	r.Signature = "TypeError: boom"
	var b bytes.Buffer
	PrintSolved(&b, r)
	out := b.String()
	if !strings.Contains(out, "\x1b[92mlooks fixed") {
		t.Errorf("keyword not bright green: %q", out)
	}
	if !strings.Contains(out, "\x1b[97mTypeError: boom") {
		t.Errorf("signature not bright: %q", out)
	}
	if !strings.Contains(out, "\x1b[36merr fix") {
		t.Errorf("command not cyan: %q", out)
	}
}

func TestTruncateLongSolution(t *testing.T) {
	long := strings.Repeat("x", 200)
	if got := truncate(long); len([]rune(got)) > maxSolutionCols {
		t.Fatalf("truncate gave %d runes", len([]rune(got)))
	}
	if got := truncate("a\nb\tc"); got != "a b c" {
		t.Fatalf("whitespace collapse: %q", got)
	}
	// CJK content is cut by display width, never mid-rune.
	cjk := truncate(strings.Repeat("类型不同，", 30))
	if !utf8.ValidString(cjk) || !strings.HasSuffix(cjk, "…") {
		t.Fatalf("CJK truncation broke a rune: %q", cjk)
	}
}
