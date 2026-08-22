package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/list"
	"github.com/wumuwutu/dejavu/internal/store"
	"github.com/wumuwutu/dejavu/internal/termx"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Browse error history (TUI; plain table when not a terminal)",
	Long: "list opens an interactive panel: navigate with ↑/↓, filter by language (l)\n" +
		"and status (s), enter for details, e to edit the solution inline\n" +
		"(enter saves, esc cancels).\n" +
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
			defer termx.PlainUnlessTTY(out)()
			printErrorTable(out, items, listAll)
			return nil
		}

		m := list.New(items)
		m.Save = st.AddFix
		p := tea.NewProgram(m)
		_, err = p.Run()
		return err
	},
}

var listAll bool

func init() {
	listCmd.Flags().BoolVar(&listAll, "all", false, "show all errors in the plain table (default: latest 20)")
	rootCmd.AddCommand(listCmd)
}

// printErrorTable is the non-TTY rendering of err list.
func printErrorTable(w io.Writer, items []store.Error, all bool) {
	shown := items
	if !all && len(shown) > defaultListLimit {
		shown = shown[:defaultListLimit]
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tLANG\tSTATUS\tSEEN\tLAST\tSIGNATURE")
	for _, e := range shown {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\t%s\n",
			e.ID, e.Language, orDash(e.Pending), e.Count,
			e.LastSeen.Format("2006-01-02"), termx.Truncate(e.Signature, 72))
	}
	tw.Flush() //nolint:errcheck
	printMoreLine(w, len(items)-len(shown), "err list --all")
}
