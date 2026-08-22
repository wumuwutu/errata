package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/wumuwutu/dejavu/internal/store"
)

func TestResolveFixTargetLatestPending(t *testing.T) {
	st := setupTestStore(t) // seeds one pending python error in /tmp/proj
	_, _, err := st.UpsertError(&store.Error{
		Fingerprint: "00000000000000f2",
		Signature:   "ModuleNotFoundError: No module named 'torch'",
		Language:    "python",
		Command:     "python train.py",
		ProjectDir:  "/tmp/proj",
	})
	if err != nil {
		t.Fatal(err)
	}

	// No argument: the most recent pending error wins, no picking.
	target, more, err := resolveFixTarget(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	if target == nil || target.Signature != "ModuleNotFoundError: No module named 'torch'" {
		t.Fatalf("target = %+v, want the latest pending error", target)
	}
	if more != 1 {
		t.Fatalf("more = %d, want 1", more)
	}

	// Explicit id still wins.
	older, more, err := resolveFixTarget(st, []string{"1"})
	if err != nil || older == nil || older.ID != 1 || more != 0 {
		t.Fatalf("by id: target=%+v more=%d err=%v", older, more, err)
	}
}

func TestResolveFixTargetNonePending(t *testing.T) {
	st := setupTestStore(t)
	if err := st.AddFix(1, "done"); err != nil { // resolve the only pending
		t.Fatal(err)
	}
	target, more, err := resolveFixTarget(st, nil)
	if err != nil || target != nil || more != 0 {
		t.Fatalf("no pending: target=%+v more=%d err=%v", target, more, err)
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
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("target summary must be one line:\n%s", out)
	}
	for _, want := range []string{"#3", "TypeError: boom", "2026-08-21", "/home/x/api"} {
		if !strings.Contains(out, want) {
			t.Errorf("target summary missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "──") || strings.Contains(out, "—") {
		t.Errorf("target summary must use ASCII dashes only: %q", out)
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
