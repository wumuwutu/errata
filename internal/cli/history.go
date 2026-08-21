package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/store"
)

var historyProject string

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "List the errors (and their fixes) one project put you through",
	Long: "history shows every error recorded under a project directory\n" +
		"(default: current directory), oldest first — the project's pit list.",
	Example: "  err history\n  err history --project ~/projects/api",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := historyProject
		if dir == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			dir = cwd
		}
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
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

		items, err := st.ListAll()
		if err != nil {
			return err
		}
		mine := filterByProject(items, dir)

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "%d errors under %s (oldest first)\n\n", len(mine), dir)
		for _, e := range mine {
			fmt.Fprintf(out, "#%-4d %-10s %-7s %-4d %s\n", e.ID,
				e.FirstSeen.Format("2006-01-02"), e.Language, e.Count, e.Signature)
			if e.Solution != "" {
				fmt.Fprintf(out, "      fix: %s\n", e.Solution)
			}
		}
		return nil
	},
}

func init() {
	historyCmd.Flags().StringVar(&historyProject, "project", "", "project directory (default: cwd)")
	rootCmd.AddCommand(historyCmd)
}

// filterByProject keeps errors recorded in dir or its subdirectories,
// ordered oldest first.
func filterByProject(items []store.Error, dir string) []store.Error {
	var out []store.Error
	for _, e := range items {
		if e.ProjectDir == dir || strings.HasPrefix(e.ProjectDir, dir+string(os.PathSeparator)) {
			out = append(out, e)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i] // ListAll is newest-first
	}
	return out
}
