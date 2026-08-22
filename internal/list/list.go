// Package list implements the err list TUI (bubbletea). All key handling,
// filtering and the inline editor state machine live in the pure Update
// method so they can be unit-tested without a terminal; persistence is
// injected as the Save function field.
package list

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wumuwutu/errata/internal/store"
	"github.com/wumuwutu/errata/internal/termx"
)

var (
	// Language and status filter cycles; "" means "all".
	langCycle   = []string{"", "python", "node", "unknown"}
	statusCycle = []string{"", "pending", "resolved", "archived"}

	headerStyle = lipgloss.NewStyle().Faint(true)
	cursorStyle = lipgloss.NewStyle().Bold(true)
	noticeStyle = lipgloss.NewStyle().Faint(true)
)

// Model is the bubbletea model for err list.
type Model struct {
	Items   []store.Error
	Lang    string // active language filter, "" = all
	Status  string // active status filter, "" = all
	Cursor  int    // index into the filtered view
	Offset  int    // first visible row (scroll window)
	Detail  bool   // detail view of the selected item
	Editing bool   // inline solution editor active
	Notice  string // transient one-line feedback (e.g. "solution saved")

	Width  int // terminal width; 0 = unknown (no row truncation)
	Height int // terminal height; 0 = unknown (render all rows)

	input textinput.Model

	// Save persists a new solution for an error. Injected by the caller.
	Save func(id int64, solution string) error
}

// New builds a Model over items (expected: most recent first).
func New(items []store.Error) Model {
	in := textinput.New()
	in.Prompt = "fix> "
	in.Placeholder = "how you fixed it"
	return Model{Items: items, input: in}
}

// Init implements tea.Model; nothing to load asynchronously.
func (m Model) Init() tea.Cmd { return nil }

// Visible returns the items passing the active filters.
func (m Model) Visible() []store.Error {
	var out []store.Error
	for _, e := range m.Items {
		if m.Lang != "" && e.Language != m.Lang {
			continue
		}
		if m.Status != "" && e.Pending != m.Status {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Update handles keys. It is pure with respect to the outside world: the
// only effect is calling the injected Save.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.Width, m.Height = ws.Width, ws.Height
		return m, nil
	}
	if m.Editing {
		return m.updateEdit(msg)
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		return m.updateKey(k)
	}
	return m, nil
}

// visibleRows is how many list rows fit on screen: the header, a scroll
// status line and the editor line (when active) reserve space. Height 0
// (unknown, e.g. in tests) means "no limit".
func (m Model) visibleRows() int {
	if m.Height <= 0 {
		return len(m.Visible())
	}
	n := m.Height - 2
	if m.Editing {
		n--
	}
	if n < 1 {
		n = 1
	}
	return n
}

// keepVisible scrolls the window so the cursor stays inside it.
func (m Model) keepVisible() Model {
	vr := m.visibleRows()
	if vr <= 0 {
		return m
	}
	if m.Cursor < m.Offset {
		m.Offset = m.Cursor
	}
	if m.Cursor >= m.Offset+vr {
		m.Offset = m.Cursor - vr + 1
	}
	return m
}

// updateEdit is the inline-editor state machine: enter saves, esc cancels,
// everything else feeds the textinput.
func (m Model) updateEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			m.Editing = false
			m.Notice = "edit cancelled"
			return m, nil
		case "enter":
			m.Editing = false
			sol := strings.TrimSpace(m.input.Value())
			if sol == "" {
				m.Notice = "empty solution — not saved"
				return m, nil
			}
			id := m.Visible()[m.Cursor].ID
			if err := m.Save(id, sol); err != nil {
				m.Notice = "save failed: " + err.Error()
				return m, nil
			}
			m.Notice = fmt.Sprintf("solution saved for error #%d", id)
			for i := range m.Items {
				if m.Items[i].ID == id {
					m.Items[i].Solution = sol
					m.Items[i].Pending = "resolved"
				}
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	vis := m.Visible()
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.Detail {
			m.Detail = false
			return m, nil
		}
		return m, tea.Quit
	case "up", "k", "w":
		if m.Cursor > 0 {
			m.Cursor--
		}
		m = m.keepVisible()
	case "down", "j", "s":
		if m.Cursor < len(vis)-1 {
			m.Cursor++
		}
		m = m.keepVisible()
	case "a":
		m.Lang = cycle(langCycle, m.Lang)
		m.Cursor = 0
		m.Offset = 0
		m.Detail = false
	case "d":
		m.Status = cycle(statusCycle, m.Status)
		m.Cursor = 0
		m.Offset = 0
		m.Detail = false
	case "enter":
		if len(vis) > 0 {
			m.Detail = true
		}
	case "e":
		if len(vis) > 0 && m.Save != nil {
			m.Editing = true
			m.input.SetValue(vis[m.Cursor].Solution)
			m.input.CursorEnd()
			return m, m.input.Focus()
		}
	}
	return m, nil
}

func cycle(options []string, cur string) string {
	for i, o := range options {
		if o == cur {
			return options[(i+1)%len(options)]
		}
	}
	return options[0]
}

// View renders the current state.
func (m Model) View() string {
	var b strings.Builder
	if m.Detail {
		b.WriteString(m.detailView())
	} else {
		b.WriteString(m.listView())
	}
	if m.Editing {
		b.WriteString(m.input.View() + "\n")
	}
	if m.Notice != "" {
		b.WriteString(noticeStyle.Render(m.Notice) + "\n")
	}
	return b.String()
}

func (m Model) listView() string {
	var b strings.Builder
	vis := m.Visible()
	fmt.Fprintf(&b, "%s\n", headerStyle.Render(fmt.Sprintf(
		"err list — %d/%d errors   lang: %s   status: %s   (w/s navigate, enter detail, e edit, a/d filter, q quit)",
		len(vis), len(m.Items), orAll(m.Lang), orAll(m.Status))))

	// Scroll window: only the rows around the cursor are rendered, so
	// hundreds of entries never overflow the screen.
	start := m.Offset
	if start > len(vis) {
		start = len(vis)
	}
	end := start + m.visibleRows()
	if end > len(vis) {
		end = len(vis)
	}
	for i := start; i < end; i++ {
		e := vis[i]
		line := fmt.Sprintf("  #%-4d %-7s %-9s %-4s %s",
			e.ID, e.Language, orAll(e.Pending), fmt.Sprintf("x%d", e.Count), e.Signature)
		if m.Width > 0 {
			line = termx.Truncate(line, m.Width)
		}
		if i == m.Cursor {
			line = cursorStyle.Render("> " + strings.TrimPrefix(line, "  "))
		}
		b.WriteString(line + "\n")
	}
	if len(vis) == 0 {
		b.WriteString("  (no errors match)\n")
	} else if start > 0 || end < len(vis) {
		fmt.Fprintf(&b, "%s\n", noticeStyle.Render(fmt.Sprintf(
			"  … showing %d-%d of %d (↑/↓ scroll)", start+1, end, len(vis))))
	}
	return b.String()
}

func (m Model) detailView() string {
	vis := m.Visible()
	if m.Cursor >= len(vis) {
		return "(gone)\n"
	}
	e := vis[m.Cursor]
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", headerStyle.Render(fmt.Sprintf("error #%d — esc back, e edit solution", e.ID)))
	fmt.Fprintf(&b, "signature:   %s\n", e.Signature)
	fmt.Fprintf(&b, "fingerprint: %s\n", e.Fingerprint)
	fmt.Fprintf(&b, "language:    %s\n", e.Language)
	fmt.Fprintf(&b, "status:      %s\n", orAll(e.Pending))
	fmt.Fprintf(&b, "seen:        %d times (first %s, last %s)\n", e.Count,
		e.FirstSeen.Format("2006-01-02 15:04"), e.LastSeen.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "command:     %s\n", e.Command)
	fmt.Fprintf(&b, "directory:   %s\n", e.ProjectDir)
	if e.GitCommit != "" {
		fmt.Fprintf(&b, "git commit:  %s\n", e.GitCommit)
	}
	fmt.Fprintf(&b, "solution:    %s\n", orDash(e.Solution))
	b.WriteString("\n── raw sample ──\n")
	b.WriteString(strings.TrimRight(e.RawSample, "\n") + "\n")
	return b.String()
}

func orAll(s string) string {
	if s == "" {
		return "all"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
