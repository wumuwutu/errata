package cli

import (
	"bytes"
	"strings"
	"testing"

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
