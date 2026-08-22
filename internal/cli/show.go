package cli

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/store"
)

var showCmd = &cobra.Command{
	Use:     "show <id>",
	Short:   "Show full details of an error record",
	Example: "  err show 3",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id %q", args[0])
		}

		dbPath, err := config.DBPath()
		if err != nil {
			return err
		}
		st, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer st.Close()

		e, err := st.Get(id)
		if err != nil {
			return err
		}
		if e == nil {
			return fmt.Errorf("error #%d not found", id)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "id:\t%d\n", e.ID)
		fmt.Fprintf(w, "fingerprint:\t%s\n", e.Fingerprint)
		fmt.Fprintf(w, "language:\t%s\n", e.Language)
		fmt.Fprintf(w, "signature:\t%s\n", e.Signature)
		fmt.Fprintf(w, "status:\t%s\n", orDash(e.Pending))
		fmt.Fprintf(w, "seen:\t%d times (first %s, last %s)\n",
			e.Count, e.FirstSeen.Format("2006-01-02 15:04:05"), e.LastSeen.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(w, "command:\t%s\n", orDash(e.Command))
		fmt.Fprintf(w, "directory:\t%s\n", orDash(e.ProjectDir))
		fmt.Fprintf(w, "git commit:\t%s\n", orDash(e.GitCommit))
		fmt.Fprintf(w, "runtime:\t%s\n", orDash(e.Runtime))
		fmt.Fprintf(w, "os:\t%s\n", orDash(e.OS))
		fmt.Fprintf(w, "solution:\t%s\n", orDash(e.Solution))
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Println("\n── raw sample ──")
		fmt.Println(e.RawSample)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
