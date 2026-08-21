// Package list implements the err list TUI (bubbletea). All key handling
// and filtering lives in the pure Update method so it can be unit-tested
// without a terminal; side effects (opening $EDITOR, saving the solution)
// are injected as function fields.
package list

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wumuwutu/dejavu/internal/store"
)

// EditFinishedMsg reports the outcome of an $EDITOR session.
type EditFinishedMsg struct {
	ErrorID  int64
	Solution string
	Err      error
}

var (
	// Language and status filter cycles; "" means "all".
	langCycle   = []string{"", "python", "node"}
	statusCycle = []string{"", "pending", "resolved", "archived"}

	headerStyle = lipgloss.NewStyle().Faint(true)
	cursorStyle = lipgloss.NewStyle().Bold(true)
	statusStyle = lipgloss.NewStyle().Faint(true)
)

// Model is the bubbletea model for err list.
type Model struct {
	Items  []store.Error
	Lang   string // active language filter, "" = all
	Status string // active status filter, "" = all
	Cursor int    // index into the filtered view
	Detail bool   // detail view of the selected item
	Notice string // transient one-line feedback (e.g. "solution saved")

	// OpenEditor opens $EDITOR on the item's current solution and returns
	// a command producing EditFinishedMsg. Injected by the caller.
	OpenEditor func(e store.Error) tea.Cmd
	// Save persists a new solution for an error. Injected by the caller.
	Save func(id int64, solution string) error
}

// New builds a Model over items (expected: most recent first).
func New(items []store.Error) Model {
	return Model{Items: items}
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
// only effects are the returned tea.Cmds built from the injected hooks.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case EditFinishedMsg:
		m.Detail = false
		switch {
		case msg.Err != nil:
			m.Notice = "editor failed: " + msg.Err.Error()
		case strings.TrimSpace(msg.Solution) == "":
			m.Notice = "empty solution — not saved"
		default:
			if err := m.Save(msg.ErrorID, msg.Solution); err != nil {
				m.Notice = "save failed: " + err.Error()
			} else {
				m.Notice = fmt.Sprintf("solution saved for error #%d", msg.ErrorID)
				for i := range m.Items {
					if m.Items[i].ID == msg.ErrorID {
						m.Items[i].Solution = msg.Solution
						m.Items[i].Pending = "resolved"
					}
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
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
	case "up", "k":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "down", "j":
		if m.Cursor < len(vis)-1 {
			m.Cursor++
		}
	case "l":
		m.Lang = cycle(langCycle, m.Lang)
		m.Cursor = 0
		m.Detail = false
	case "s":
		m.Status = cycle(statusCycle, m.Status)
		m.Cursor = 0
		m.Detail = false
	case "enter":
		if len(vis) > 0 {
			m.Detail = true
		}
	case "e":
		if len(vis) > 0 && m.OpenEditor != nil {
			return m, m.OpenEditor(vis[m.Cursor])
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
	if m.Detail {
		return m.detailView()
	}
	return m.listView()
}

func (m Model) listView() string {
	var b strings.Builder
	vis := m.Visible()
	fmt.Fprintf(&b, "%s\n", headerStyle.Render(fmt.Sprintf(
		"err list — %d/%d errors   lang: %s   status: %s   (↑↓ navigate, enter detail, e edit, l/s filter, q quit)",
		len(vis), len(m.Items), orAll(m.Lang), orAll(m.Status))))
	for i, e := range vis {
		line := fmt.Sprintf("  #%-4d %-7s %-9s %-4s %s",
			e.ID, e.Language, orAll(e.Pending), fmt.Sprintf("x%d", e.Count), e.Signature)
		if i == m.Cursor {
			line = cursorStyle.Render("> " + strings.TrimPrefix(line, "  "))
		}
		b.WriteString(line + "\n")
	}
	if len(vis) == 0 {
		b.WriteString("  (no errors match)\n")
	}
	if m.Notice != "" {
		b.WriteString(statusStyle.Render(m.Notice) + "\n")
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
