package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestDeleteYesFlag(t *testing.T) {
	setupTestStore(t)
	var buf bytes.Buffer
	deleteCmd.SetErr(&buf)
	defer deleteCmd.SetErr(nil)

	deleteYes = true
	defer func() { deleteYes = false }()
	if err := deleteCmd.RunE(deleteCmd, []string{"1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "deleted error #1") {
		t.Fatalf("output: %q", buf.String())
	}

	// Gone for good.
	if err := deleteCmd.RunE(deleteCmd, []string{"1"}); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Fatalf("second delete must report not found: %v", err)
	}
}

func TestDeleteNonTTYRefuses(t *testing.T) {
	setupTestStore(t)
	deleteYes = false
	// stdin is not a terminal in tests: refuse without --yes.
	if err := deleteCmd.RunE(deleteCmd, []string{"1"}); err == nil ||
		!strings.Contains(err.Error(), "--yes") {
		t.Fatalf("must refuse without --yes: %v", err)
	}
}

func TestClearYesFlag(t *testing.T) {
	st := setupTestStore(t)
	var buf bytes.Buffer
	clearCmd.SetErr(&buf)
	defer clearCmd.SetErr(nil)

	clearYes = true
	defer func() { clearYes = false }()
	if err := clearCmd.RunE(clearCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "cleared 1 error records") {
		t.Fatalf("output: %q", buf.String())
	}
	if items, _ := st.ListAll(); len(items) != 0 {
		t.Fatalf("records survived clear: %d", len(items))
	}

	// Clearing an empty library is a no-op message.
	buf.Reset()
	if err := clearCmd.RunE(clearCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "nothing to clear") {
		t.Fatalf("output: %q", buf.String())
	}
}

func TestClearNonTTYRefuses(t *testing.T) {
	setupTestStore(t)
	clearYes = false
	if err := clearCmd.RunE(clearCmd, nil); err == nil ||
		!strings.Contains(err.Error(), "--yes") {
		t.Fatalf("must refuse without --yes: %v", err)
	}
}
