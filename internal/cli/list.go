package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/list"
	"github.com/wumuwutu/dejavu/internal/store"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Browse error history (TUI; plain table when not a terminal)",
	Long: "list opens an interactive panel: navigate with ↑/↓, filter by language (l)\n" +
		"and status (s), enter for details, e to edit the solution in $EDITOR.\n" +
		"When stdout is not a terminal it prints a plain table instead.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, err := config.DBPath()
		if err != nil {
			return err
		}
		st, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer st.Close()

		items, err := st.ListAll()
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		if f, isFile := out.(*os.File); !isFile || !term.IsTerminal(int(f.Fd())) {
			// Non-TTY fallback: pipes and scripts get a plain table.
			printErrorTable(out, items)
			return nil
		}

		m := list.New(items)
		m.Save = st.AddFix
		m.OpenEditor = openSolutionEditor
		p := tea.NewProgram(m)
		_, err = p.Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}

// printErrorTable is the non-TTY rendering of err list.
func printErrorTable(w io.Writer, items []store.Error) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tLANG\tSTATUS\tSEEN\tLAST\tSIGNATURE")
	for _, e := range items {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\t%s\n",
			e.ID, e.Language, orDash(e.Pending), e.Count,
			e.LastSeen.Format("2006-01-02"), e.Signature)
	}
	tw.Flush() //nolint:errcheck
}

// openSolutionEditor opens $EDITOR on a temp file seeded with the current
// solution; saving yields an EditFinishedMsg.
func openSolutionEditor(e store.Error) tea.Cmd {
	path := solutionTmpPath(e.ID)
	if err := os.WriteFile(path, []byte(e.Solution), 0o600); err != nil {
		return func() tea.Msg { return list.EditFinishedMsg{ErrorID: e.ID, Err: err} }
	}
	return tea.ExecProcess(solutionEditorCmd(path), func(err error) tea.Msg {
		defer os.Remove(path) //nolint:errcheck
		if err != nil {
			return list.EditFinishedMsg{ErrorID: e.ID, Err: err}
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return list.EditFinishedMsg{ErrorID: e.ID, Err: rerr}
		}
		return list.EditFinishedMsg{ErrorID: e.ID, Solution: string(data)}
	})
}

func solutionEditorCmd(path string) *exec.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	return exec.Command(editor, path)
}

func solutionTmpPath(id int64) string {
	return fmt.Sprintf("%s%cdejavu-solution-%d.txt", os.TempDir(), os.PathSeparator, id)
}
