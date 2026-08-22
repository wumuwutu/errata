package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeBuffer(t *testing.T, content []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sess.err")
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func sentinel(seq int64) string {
	return fmt.Sprintf("%s%d\a", sentinelPrefix, seq)
}

func TestReadStderrDeltaSentinel(t *testing.T) {
	stale := "prompt$ vim demo.py \r\npartial echo of earlier typing\r\n"
	stderr := "Traceback ...\nAttributeError: boom\n"

	// Straggler bytes from earlier commands precede the sentinel; only the
	// bytes after this command's sentinel are its stderr.
	buf := []byte("older stuff\n" + stale + sentinel(7) + stderr)
	f := writeBuffer(t, buf)
	got := string(readStderrDelta(f, 0, 7))
	if got != stderr {
		t.Fatalf("delta = %q, want %q", got, stderr)
	}

	// Offset narrows the read window; the sentinel still delimits.
	off := int64(len("older stuff\n"))
	got = string(readStderrDelta(f, off, 7))
	if got != stderr {
		t.Fatalf("offset window: delta = %q, want %q", got, stderr)
	}

	// A sentinel for a DIFFERENT seq (previous command arriving late) must
	// not be accepted: better to skip than to misattribute.
	got = string(readStderrDelta(f, off, 8))
	if got != "" {
		t.Fatalf("wrong seq must yield empty delta, got %q", got)
	}

	// No sentinel in the window at all: empty.
	f2 := writeBuffer(t, []byte("just some output\n"))
	if got := readStderrDelta(f2, 0, 3); len(got) != 0 {
		t.Fatalf("missing sentinel must yield empty delta, got %q", got)
	}

	// seq == 0: legacy pre-sentinel hook — raw offset slice (old behavior).
	if got := string(readStderrDelta(f2, 0, 0)); got != "just some output\n" {
		t.Fatalf("legacy fallback = %q", got)
	}
}

// TestReadStderrDeltaLegacyPayload: hooks installed before the product
// rename emit the sentinel with the OLD payload word. Parsing keys on the
// sequence number alone, so those sessions keep recording until restart.
func TestReadStderrDeltaLegacyPayload(t *testing.T) {
	legacy := "\x1b]6973;oldproductname;4\a" // any payload word, same OSC code + seq
	stderr := "TypeError: keeps working\n"
	f := writeBuffer(t, []byte("junk before\n"+legacy+stderr))
	if got := string(readStderrDelta(f, 0, 4)); got != stderr {
		t.Fatalf("legacy payload word not accepted: %q", got)
	}
	// A different seq still must not match.
	if got := readStderrDelta(f, 0, 5); len(got) != 0 {
		t.Fatalf("wrong seq must yield empty delta, got %q", got)
	}
}
