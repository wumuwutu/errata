package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/store"
)

// setupTestStore points the config/data paths at a temp dir and seeds one
// error record.
func setupTestStore(t *testing.T) *store.Store {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	_, _, err = st.UpsertError(&store.Error{
		Fingerprint: "00000000000000f1",
		Signature:   "TypeError: cannot add <VAL>",
		Language:    "python",
		Command:     "python app.py",
		ProjectDir:  "/tmp/proj",
	})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestListNonTTYFallback: with stdout not a terminal (here: a buffer),
// err list must print the plain table, not launch the TUI.
func TestListNonTTYFallback(t *testing.T) {
	setupTestStore(t)

	var buf bytes.Buffer
	listCmd.SetOut(&buf)
	defer listCmd.SetOut(nil)

	if err := listCmd.RunE(listCmd, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "TypeError: cannot add <VAL>") {
		t.Fatalf("table missing signature:\n%s", out)
	}
	if !strings.Contains(out, "ID\t") && !strings.Contains(out, "ID") {
		t.Fatalf("table header missing:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatal("non-TTY output must not contain ANSI escapes")
	}
}

func TestFilterByProject(t *testing.T) {
	mk := func(id int64, dir string, day time.Time) store.Error {
		return store.Error{ID: id, ProjectDir: dir, FirstSeen: day}
	}
	now := time.Now()
	// ListAll order is newest-first; filterByProject must return
	// oldest-first and include subdirectories only.
	items := []store.Error{
		mk(3, "/proj/sub", now),
		mk(2, "/proj", now.Add(-time.Hour)),
		mk(9, "/projectx", now.Add(-2*time.Hour)), // prefix-but-not-subdir
		mk(1, "/proj", now.Add(-3*time.Hour)),
	}
	got := filterByProject(items, "/proj")
	if len(got) != 3 {
		t.Fatalf("got %d items, want 3", len(got))
	}
	for i, wantID := range []int64{1, 2, 3} {
		if got[i].ID != wantID {
			t.Fatalf("order[%d] = %d, want %d", i, got[i].ID, wantID)
		}
	}
}

// TestPrintErrorTableTruncatesCJK: long CJK signatures are cut by display
// width at a rune boundary, never mid-character.
func TestPrintErrorTableTruncatesCJK(t *testing.T) {
	long := store.Error{
		ID:        7,
		Language:  "python",
		Pending:   "pending",
		Count:     2,
		LastSeen:  time.Now(),
		Signature: "类型错误：" + strings.Repeat("类型不同需要显式转换，", 20),
	}
	var buf bytes.Buffer
	printErrorTable(&buf, []store.Error{long}, true)
	out := buf.String()
	if !utf8.ValidString(out) {
		t.Fatalf("table output is not valid UTF-8:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("long CJK signature must be ellipsized:\n%s", out)
	}
	if strings.Contains(out, strings.Repeat("类型不同需要显式转换，", 20)) {
		t.Fatalf("signature not truncated:\n%s", out)
	}
}

// seedMany inserts n errors with distinct fingerprints.
func seedMany(t *testing.T, st *store.Store, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		_, _, err := st.UpsertError(&store.Error{
			Fingerprint: fmt.Sprintf("000000000000a%03x", i),
			Signature:   fmt.Sprintf("TypeError: boom %d", i),
			Language:    "python",
			ProjectDir:  "/tmp/proj",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPrintErrorTableLimit(t *testing.T) {
	st := setupTestStore(t)
	seedMany(t, st, 25) // 26 total
	items, err := st.ListAll()
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	printErrorTable(&buf, items, false)
	out := buf.String()
	if got := strings.Count(out, "TypeError: boom"); got != 20 {
		t.Fatalf("default table shows %d rows, want 20", got)
	}
	if !strings.Contains(out, "and 6 more (err list --all)") {
		t.Fatalf("missing overflow footer:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatal("non-TTY output must stay plain")
	}

	buf.Reset()
	printErrorTable(&buf, items, true)
	if got := strings.Count(buf.String(), "TypeError: boom"); got != 25 {
		t.Fatalf("--all table shows %d probe rows, want 25", got)
	}
}

func TestPendingLimit(t *testing.T) {
	st := setupTestStore(t)
	seedMany(t, st, 25)

	var buf bytes.Buffer
	pendingCmd.SetOut(&buf)
	defer pendingCmd.SetOut(nil)

	pendingAll = false
	if err := pendingCmd.RunE(pendingCmd, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// The seeded row is the oldest and falls outside the latest-20 window.
	if got := strings.Count(out, "TypeError: boom"); got != 20 {
		t.Fatalf("pending default shows %d probe rows, want 20", got)
	}
	if !strings.Contains(out, "and 6 more (err pending --all)") {
		t.Fatalf("missing overflow footer:\n%s", out)
	}

	buf.Reset()
	pendingAll = true
	defer func() { pendingAll = false }()
	if err := pendingCmd.RunE(pendingCmd, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(buf.String(), "TypeError: boom"); got != 25 {
		t.Fatalf("pending --all shows %d probe rows, want 25", got)
	}
}
