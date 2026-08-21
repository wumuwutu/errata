package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/store"
)

var pendingCmd = &cobra.Command{
	Use:     "pending",
	Short:   "List unresolved errors and the record rate",
	Example: "  err pending",
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
		if len(items) == 0 {
			fmt.Println("no pending errors")
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tLANG\tSEEN\tFIRST\tLAST\tSIGNATURE")
			for _, it := range items {
				fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s\n",
					it.ErrorID, it.Language, it.Count,
					it.FirstSeen.Format("2006-01-02"), it.LastSeen.Format("2006-01-02"),
					it.Signature)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			fmt.Println("\nrecord a solution with: err fix <id>")
		}

		resolved, total, err := st.RecordRate()
		if err != nil {
			return err
		}
		rate := 0.0
		if total > 0 {
			rate = float64(resolved) / float64(total) * 100
		}
		fmt.Printf("\nrecord rate: %d/%d errors have a solution (%.0f%%)\n", resolved, total, rate)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pendingCmd)
}
