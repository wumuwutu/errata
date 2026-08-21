package list

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wumuwutu/dejavu/internal/store"
)

func items() []store.Error {
	return []store.Error{
		{ID: 1, Signature: "TypeError: a", Language: "python", Pending: "pending", Count: 3, LastSeen: time.Now()},
		{ID: 2, Signature: "Error: b", Language: "node", Pending: "resolved", Count: 1, Solution: "fixed"},
		{ID: 3, Signature: "KeyError: c", Language: "python", Pending: "archived", Count: 9},
	}
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestNavigationClamps(t *testing.T) {
	m := New(items())
	m2, _ := m.Update(key("down"))
	m3, _ := m2.(Model).Update(key("down"))
	if m3.(Model).Cursor != 2 {
		t.Fatalf("cursor = %d, want 2", m3.(Model).Cursor)
	}
	m4, _ := m3.(Model).Update(key("down")) // clamp at bottom
	if m4.(Model).Cursor != 2 {
		t.Fatalf("cursor = %d, want clamped 2", m4.(Model).Cursor)
	}
	m5, _ := m4.(Model).Update(key("up"))
	m6, _ := m5.(Model).Update(key("up"))
	m7, _ := m6.(Model).Update(key("up")) // clamp at top
	if m7.(Model).Cursor != 0 {
		t.Fatalf("cursor = %d, want clamped 0", m7.(Model).Cursor)
	}
}

func TestFilters(t *testing.T) {
	m := New(items())
	if got := len(m.Visible()); got != 3 {
		t.Fatalf("visible = %d, want 3", got)
	}
	// l: all -> python
	m1, _ := m.Update(key("l"))
	if mv := m1.(Model); mv.Lang != "python" || len(mv.Visible()) != 2 {
		t.Fatalf("lang=%q visible=%d", mv.Lang, len(mv.Visible()))
	}
	// s: all -> pending; combined with python: only #1
	m2, _ := m1.(Model).Update(key("s"))
	if mv := m2.(Model); mv.Status != "pending" || len(mv.Visible()) != 1 {
		t.Fatalf("status=%q visible=%d", mv.Status, len(mv.Visible()))
	}
	// cycling status: pending -> resolved -> archived -> all
	m3, _ := m2.(Model).Update(key("s"))
	m4, _ := m3.(Model).Update(key("s"))
	m5, _ := m4.(Model).Update(key("s"))
	if mv := m5.(Model); mv.Status != "" {
		t.Fatalf("status cycle should return to all, got %q", mv.Status)
	}
}

func TestDetailEnterEsc(t *testing.T) {
	m := New(items())
	m1, _ := m.Update(key("enter"))
	if !m1.(Model).Detail {
		t.Fatal("enter should open detail")
	}
	if v := m1.(Model).View(); !strings.Contains(v, "TypeError: a") || !strings.Contains(v, "esc back") {
		t.Fatalf("detail view wrong:\n%s", v)
	}
	m2, _ := m1.(Model).Update(key("esc"))
	if m2.(Model).Detail {
		t.Fatal("esc should go back")
	}
}

func TestEditFinishedSaves(t *testing.T) {
	var savedID int64
	var savedSol string
	m := New(items())
	m.Save = func(id int64, sol string) error { savedID, savedSol = id, sol; return nil }

	m1, _ := m.Update(EditFinishedMsg{ErrorID: 1, Solution: "new fix"})
	mv := m1.(Model)
	if savedID != 1 || savedSol != "new fix" {
		t.Fatalf("Save got (%d, %q)", savedID, savedSol)
	}
	if mv.Items[0].Solution != "new fix" || mv.Items[0].Pending != "resolved" {
		t.Fatalf("item not updated in place: %+v", mv.Items[0])
	}
	if !strings.Contains(mv.Notice, "#1") {
		t.Fatalf("notice = %q", mv.Notice)
	}
}

func TestEditFinishedFailure(t *testing.T) {
	m := New(items())
	m.Save = func(int64, string) error { return errors.New("db gone") }
	m1, _ := m.Update(EditFinishedMsg{ErrorID: 1, Solution: "x"})
	if !strings.Contains(m1.(Model).Notice, "save failed") {
		t.Fatalf("notice = %q", m1.(Model).Notice)
	}
	m2, _ := m.Update(EditFinishedMsg{ErrorID: 1, Solution: "  "})
	if !strings.Contains(m2.(Model).Notice, "empty") {
		t.Fatalf("notice = %q", m2.(Model).Notice)
	}
}

func TestEmptyList(t *testing.T) {
	m := New(nil)
	m1, _ := m.Update(key("enter"))
	if m1.(Model).Detail {
		t.Fatal("enter on empty list must not open detail")
	}
	m2, _ := m.Update(key("e"))
	if _, isModel := m2.(Model); !isModel {
		t.Fatal("e on empty list should be a no-op")
	}
}
