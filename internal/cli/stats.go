package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/store"
	"github.com/wumuwutu/dejavu/internal/termx"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Error distribution — a mirror for your debugging habits",
	Args:  cobra.NoArgs,
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

		s, err := st.Stats(time.Now(), 5, 5)
		if err != nil {
			return err
		}
		printStats(cmd.OutOrStdout(), s)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}

func printStats(w io.Writer, s *store.Stats) {
	rate := 0.0
	if s.Total > 0 {
		rate = float64(s.Resolved) / float64(s.Total) * 100
	}
	fmt.Fprintf(w, "errors: %d total, %d with a solution (%.0f%% record rate)\n", s.Total, s.Resolved, rate)

	fmt.Fprintln(w, "\nby language:")
	writeKVs(w, s.ByLanguage)

	fmt.Fprintln(w, "\ntop projects (by occurrences):")
	writeKVs(w, s.ByProject)

	fmt.Fprintln(w, "\nmost repeated:")
	for _, kv := range s.TopRepeated {
		fmt.Fprintf(w, "  %4dx  %s\n", kv.N, termx.Truncate(kv.Label, 60))
	}

	fmt.Fprintln(w, "\nnew errors per week (oldest → latest):")
	for i, n := range s.WeeklyNew {
		fmt.Fprintf(w, "  week -%d  %s %d\n", len(s.WeeklyNew)-1-i, strings.Repeat("█", n), n)
	}
}

func writeKVs(w io.Writer, kvs []store.KV) {
	if len(kvs) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}
	for _, kv := range kvs {
		// Pad by display cells, not bytes, so CJK labels stay aligned.
		fmt.Fprintf(w, "  %s %d\n", termx.PadRight(termx.Truncate(kv.Label, 30), 32), kv.N)
	}
}
