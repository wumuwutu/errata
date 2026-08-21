package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/wumuwutu/dejavu/internal/store"
)

func TestParseChoice(t *testing.T) {
	cases := []struct {
		in      string
		n       int
		wantIdx int
		wantOK  bool
	}{
		{"\n", 3, 0, true},   // empty = first
		{"2\n", 3, 1, true},  // valid pick
		{"3", 3, 2, true},    // no newline
		{"0\n", 3, 0, false}, // out of range low
		{"4\n", 3, 0, false}, // out of range high
		{"x\n", 3, 0, false}, // not a number
	}
	for _, c := range cases {
		idx, ok := parseChoice(c.in, c.n)
		if idx != c.wantIdx || ok != c.wantOK {
			t.Errorf("parseChoice(%q, %d) = (%d, %v), want (%d, %v)",
				c.in, c.n, idx, ok, c.wantIdx, c.wantOK)
		}
	}
}

func TestPrintFixTarget(t *testing.T) {
	e := &store.Error{
		ID:         3,
		Signature:  "TypeError: boom",
		Count:      5,
		LastSeen:   time.Date(2026, 8, 21, 10, 4, 0, 0, time.UTC),
		ProjectDir: "/home/x/api",
	}
	var b bytes.Buffer
	printFixTarget(&b, e)
	out := b.String()
	for _, want := range []string{"#3", "TypeError: boom", "5 times", "2026-08-21", "/home/x/api"} {
		if !strings.Contains(out, want) {
			t.Errorf("target summary missing %q:\n%s", want, out)
		}
	}
}

func TestReadSolutionFlagAndPipe(t *testing.T) {
	if s, err := readSolution(strings.NewReader(""), &bytes.Buffer{}, "  direct fix  "); err != nil || s != "direct fix" {
		t.Fatalf("flag: %q %v", s, err)
	}
	if s, err := readSolution(strings.NewReader("piped fix\n"), &bytes.Buffer{}, ""); err != nil || s != "piped fix" {
		t.Fatalf("pipe: %q %v", s, err)
	}
	if _, err := readSolution(strings.NewReader("\n"), &bytes.Buffer{}, ""); err == nil {
		t.Fatal("empty piped solution must be rejected")
	}
}
