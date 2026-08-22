package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/store"
	"github.com/wumuwutu/dejavu/internal/termx"
)

// defaultListLimit caps how many rows list-style commands print unless
// --all is given (long histories must not flood the terminal).
const defaultListLimit = 20

var pendingAll bool

var pendingCmd = &cobra.Command{
	Use:     "pending",
	Short:   "List unresolved errors and the record rate",
	Example: "  err pending\n  err pending --all",
	Args:    cobra.NoArgs,
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

		items, err := st.ListPending()
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		defer termx.PlainUnlessTTY(out)()
		if len(items) == 0 {
			fmt.Fprintln(out, "no pending errors")
		} else {
			shown := items
			if !pendingAll && len(shown) > defaultListLimit {
				shown = shown[:defaultListLimit]
			}
			w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tLANG\tSEEN\tFIRST\tLAST\tSIGNATURE")
			for _, it := range shown {
				fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s\n",
					it.ErrorID, it.Language, it.Count,
					it.FirstSeen.Format("2006-01-02"), it.LastSeen.Format("2006-01-02"),
					termx.Truncate(it.Signature, 72))
			}
			if err := w.Flush(); err != nil {
				return err
			}
			printMoreLine(out, len(items)-len(shown), "err pending --all")
			fmt.Fprintln(out, "\n"+termx.Faint("--err-- record a solution with: ")+termx.Cyan("err fix")+termx.Faint(" <id>"))
		}

		resolved, total, err := st.RecordRate()
		if err != nil {
			return err
		}
		rate := 0.0
		if total > 0 {
			rate = float64(resolved) / float64(total) * 100
		}
		fmt.Fprintf(out, "\nrecord rate: %d/%d errors have a solution (%.0f%%)\n", resolved, total, rate)
		return nil
	},
}

// printMoreLine tells the user how many rows were elided by the default
// limit and how to see them. hidden <= 0 prints nothing.
func printMoreLine(out io.Writer, hidden int, allCmd string) {
	if hidden <= 0 {
		return
	}
	fmt.Fprintf(out, "%s%d%s\n",
		termx.Faint("--err-- … and "), hidden,
		termx.Faint(" more (")+termx.Cyan(allCmd)+termx.Faint(")"))
}

func init() {
	pendingCmd.Flags().BoolVar(&pendingAll, "all", false, "show all pending errors (default: latest 20)")
	rootCmd.AddCommand(pendingCmd)
}
