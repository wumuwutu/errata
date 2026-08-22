package termx

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

func TestTruncateRuneSafe(t *testing.T) {
	// ASCII passthrough and cut.
	if got := Truncate("hello", 10); got != "hello" {
		t.Fatalf("passthrough: %q", got)
	}
	if got := Truncate("hello world", 8); got != "hello w…" {
		t.Fatalf("ascii cut: %q", got)
	}

	// CJK: 2 cells per rune — the cut must land on a rune boundary.
	s := "类型不同：左边是 int 右边是 str"
	got := Truncate(s, 20)
	if !strings.HasSuffix(got, "…") || !utf8.ValidString(got) {
		t.Fatalf("bad cut: %q", got)
	}
	if w := runewidth.StringWidth(got); w > 20 {
		t.Fatalf("width %d > 20: %q", w, got)
	}
	for _, r := range strings.TrimSuffix(got, "…") {
		if !strings.ContainsRune(s, r) {
			t.Fatalf("rune %q not from the original: %q", r, got)
		}
	}

	// Emoji and mixed content.
	mixed := "装包即可：pip install torch 🚀🚀🚀🚀🚀"
	got = Truncate(mixed, 24)
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "…") {
		t.Fatalf("mixed cut: %q", got)
	}
	if runewidth.StringWidth(got) > 24 {
		t.Fatalf("mixed width: %q", got)
	}

	// Zero/negative cap means "no limit".
	if got := Truncate(s, 0); got != s {
		t.Fatalf("cap 0 must not cut: %q", got)
	}
}

func TestPaletteNoColor(t *testing.T) {
	defer func() { NoColor = false }()
	NoColor = false
	if got := Faint("x"); !strings.HasPrefix(got, faint) || !strings.HasSuffix(got, reset) {
		t.Fatalf("colored faint: %q", got)
	}
	NoColor = true
	if Faint("--err-- seen") != "--err-- seen" || Cyan("err fix") != "err fix" ||
		Green("looks fixed") != "looks fixed" || Bright("TypeError: boom") != "TypeError: boom" {
		t.Fatal("NoColor must pass text through unchanged")
	}
}

func TestShortenHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := ShortenHome(home + "/proj/x"); got != "~/proj/x" {
		t.Fatalf("got %q", got)
	}
	if got := ShortenHome(home); got != "~" {
		t.Fatalf("home itself: %q", got)
	}
	if got := ShortenHome("/elsewhere"); got != "/elsewhere" {
		t.Fatalf("outside home: %q", got)
	}
}

func TestPaletteReds(t *testing.T) {
	defer func() { NoColor = false }()
	NoColor = false
	if got := Red("x"); !strings.HasPrefix(got, red) {
		t.Fatalf("Red: %q", got)
	}
	if got := BrightRed("x"); !strings.HasPrefix(got, brightRed) || !strings.HasSuffix(got, reset) {
		t.Fatalf("BrightRed: %q", got)
	}
	NoColor = true
	if BrightRed("clear ALL?") != "clear ALL?" {
		t.Fatal("BrightRed must pass text through under NoColor")
	}
}
