package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/store"
)

// TestRecordFailureRedactsSecrets: credentials in stderr must never reach
// the database — neither in the raw sample nor in the derived signature.
func TestRecordFailureRedactsSecrets(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	stderr := "Traceback (most recent call last):\n  File \"/x/a.py\", line 1, in <module>\n" +
		"ConnectionError: dial https://alice:s3cret-pass@db.internal:5432/app with password=hunter2 failed\n"
	recordFailure("python a.py", "/tmp/proj", []byte(stderr), nil, io.Discard)

	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	items, err := st.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d records, want 1", len(items))
	}
	e := items[0]
	for _, leak := range []string{"s3cret-pass", "hunter2"} {
		if strings.Contains(e.RawSample, leak) || strings.Contains(e.Signature, leak) {
			t.Errorf("secret %q leaked:\nsig: %s\nraw: %s", leak, e.Signature, e.RawSample)
		}
	}
	if !strings.Contains(e.Signature, "***") {
		t.Errorf("signature must show the mask: %q", e.Signature)
	}
}
