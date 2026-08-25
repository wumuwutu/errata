package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/store"
)

func TestBlockSplitter(t *testing.T) {
	var sp blockSplitter
	// Lines accumulate; a blank line ends the block.
	if _, ok := sp.feed("line a"); ok {
		t.Fatal("single line must not finish a block")
	}
	if _, ok := sp.feed("line b"); ok {
		t.Fatal("still accumulating")
	}
	if block, ok := sp.feed(""); !ok || block != "line a\nline b" {
		t.Fatalf("blank line: %q %v", block, ok)
	}
	// A new error marker closes the open block first (no blank line in
	// real logs between tracebacks).
	sp.feed("Traceback (most recent call last):")
	sp.feed(`  File "/x/a.py", line 1, in <module>`)
	block, ok := sp.feed("Traceback (most recent call last):")
	if !ok || block != "Traceback (most recent call last):\n  File \"/x/a.py\", line 1, in <module>" {
		t.Fatalf("marker split: %q %v", block, ok)
	}
	// EOF without a trailing blank line: flush returns the remainder.
	if block, ok := sp.flush(); !ok || block != "Traceback (most recent call last):" {
		t.Fatalf("flush: %q %v", block, ok)
	}
}

const watchPyTrace = `Traceback (most recent call last):
  File "/srv/app/main.py", line 9, in <module>
    boom()
TypeError: stream went boom
`

const watchJavaExc = `Exception in thread "main" java.lang.ArithmeticException: / by zero
	at com.app.Calc.div(Calc.java:7)
`

// isolateStore points the DB/config at fresh temp dirs (no seeding).
func isolateStore(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestWatchStreamStdin(t *testing.T) {
	isolateStore(t)
	var buf bytes.Buffer

	// echo traceback | err watch — a pipe ends at EOF on its own.
	n, err := watchStream(context.Background(), strings.NewReader(watchPyTrace), "stdin", "/tmp/proj", nil, &buf)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v, want 1 captured", n, err)
	}

	st := openTestStore(t)
	items, err := st.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d records, want 1", len(items))
	}
	e := items[0]
	if e.Language != "python" || e.Signature != "TypeError: stream went boom" {
		t.Fatalf("record: lang=%q sig=%q", e.Language, e.Signature)
	}
	if e.Command != "watch: stdin" {
		t.Fatalf("command = %q", e.Command)
	}

	// Second identical stream: the error is known — the hit hint prints.
	buf.Reset()
	if _, err := watchStream(context.Background(), strings.NewReader(watchPyTrace), "stdin", "/tmp/proj", nil, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "--err--") {
		t.Fatalf("repeat occurrence must print the hit hint, got %q", buf.String())
	}
}

// TestWatchFileCapturesAppendedErrors tails a temp file while another
// writer appends a Python traceback and (twice) a Java exception, the way
// `err watch /var/log/myapp.log` is used for real.
func TestWatchFileCapturesAppendedErrors(t *testing.T) {
	isolateStore(t)
	log := filepath.Join(t.TempDir(), "app.log")

	// History written before watch starts must NOT be replayed.
	if err := os.WriteFile(log, []byte(watchPyTrace), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var buf bytes.Buffer
	done := make(chan int, 1)
	go func() {
		n, err := watchFile(ctx, log, "/tmp/proj", nil, &buf)
		if err != nil {
			t.Errorf("watchFile: %v", err)
		}
		done <- n
	}()

	time.Sleep(100 * time.Millisecond) // let watch reach the file's end
	appendLog := func(s string) {
		f, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	appendLog(watchJavaExc + "\n")
	appendLog("\n" + watchPyTrace + "\n")
	appendLog(watchJavaExc) // no trailing blank line: relies on the idle flush

	st := openTestStore(t)
	deadline := time.Now().Add(5 * time.Second)
	var items []store.Error
	for time.Now().Before(deadline) {
		var err error
		items, err = st.ListAll()
		if err != nil {
			t.Fatal(err)
		}
		// All three appends landed once the java record's count is 2.
		total := 0
		for _, e := range items {
			total += e.Count
		}
		if len(items) == 2 && total == 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	n := <-done

	if len(items) != 2 {
		t.Fatalf("got %d records, want 2 (python + java): %+v", len(items), items)
	}
	if n != 3 {
		t.Fatalf("captured = %d, want 3 (java twice, python once)", n)
	}
	var py, java *store.Error
	for i := range items {
		switch items[i].Language {
		case "python":
			py = &items[i]
		case "java":
			java = &items[i]
		}
	}
	if py == nil || java == nil {
		t.Fatalf("want one python and one java record: %+v", items)
	}
	if py.Signature != "TypeError: stream went boom" {
		t.Fatalf("python sig = %q", py.Signature)
	}
	if java.Signature != "java.lang.ArithmeticException: / by zero" || java.Count != 2 {
		t.Fatalf("java record: sig=%q count=%d", java.Signature, java.Count)
	}
}
