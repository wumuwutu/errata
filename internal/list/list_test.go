package list

import (
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

func TestInlineEditSave(t *testing.T) {
	var savedID int64
	var savedSol string
	m := New(items())
	m.Save = func(id int64, sol string) error { savedID, savedSol = id, sol; return nil }

	// e enters edit mode with the current solution prefilled.
	tm, cmd := m.Update(key("e"))
	m = tm.(Model)
	if !m.Editing {
		t.Fatal("e should enter edit mode")
	}
	if cmd == nil {
		t.Fatal("entering edit mode should focus the input")
	}
	if m.input.Value() != "" {
		t.Fatalf("item #1 has no solution, input should start empty, got %q", m.input.Value())
	}

	// Type the solution, then save with enter.
	tm, _ = m.Update(key("x"))
	m = tm.(Model)
	tm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(Model)
	if m.Editing {
		t.Fatal("enter should leave edit mode")
	}
	if savedID != 1 || savedSol != "x" {
		t.Fatalf("Save got (%d, %q)", savedID, savedSol)
	}
	if m.Items[0].Solution != "x" || m.Items[0].Pending != "resolved" {
		t.Fatalf("item not updated in place: %+v", m.Items[0])
	}
	if !strings.Contains(m.Notice, "#1") {
		t.Fatalf("notice = %q", m.Notice)
	}
}

func TestInlineEditPrefillAndCancel(t *testing.T) {
	m := New(items())
	m.Save = func(int64, string) error { t.Fatal("Save must not be called"); return nil }

	// Select item #2 (has a solution), open editor: prefilled.
	tm, _ := m.Update(key("down"))
	m = tm.(Model)
	tm, _ = m.Update(key("e"))
	m = tm.(Model)
	if m.input.Value() != "fixed" {
		t.Fatalf("input prefilled with %q, want %q", m.input.Value(), "fixed")
	}

	// esc cancels without saving.
	tm, _ = m.Update(key("esc"))
	m = tm.(Model)
	if m.Editing || m.Notice != "edit cancelled" {
		t.Fatalf("after esc: editing=%v notice=%q", m.Editing, m.Notice)
	}
}

func TestInlineEditEmptyNotSaved(t *testing.T) {
	m := New(items())
	m.Save = func(int64, string) error { t.Fatal("Save must not be called"); return nil }
	tm, _ := m.Update(key("e"))
	m = tm.(Model)
	tm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // empty input
	m = tm.(Model)
	if !strings.Contains(m.Notice, "empty") {
		t.Fatalf("notice = %q", m.Notice)
	}
}

func TestEmptyList(t *testing.T) {
	m := New(nil)
	m1, _ := m.Update(key("enter"))
	if m1.(Model).Detail {
		t.Fatal("enter on empty list must not open detail")
	}
	m2, _ := m.Update(key("e"))
	if m2.(Model).Editing {
		t.Fatal("e on empty list must not start editing")
	}
}
