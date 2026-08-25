package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/store"
)

var exportOutput string

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the error library as a Markdown document",
	Long: "Export writes the whole error library to a Markdown file, grouped by project.\n" +
		"Read-only: the database is never touched. Default target is\n" +
		"./errata-export-<date>.md; --output picks a file or a directory.",
	Example: "  err export\n  err export --output ~/backups/\n  err export --output report.md",
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

		items, err := st.ListAll()
		if err != nil {
			return err
		}

		path, err := resolveExportPath(exportOutput, time.Now())
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(renderExport(items, time.Now())), 0o644); err != nil {
			return fmt.Errorf("cannot write %s: %w", path, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "exported %d errors to %s\n", len(items), path)
		return nil
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output file or directory")
	rootCmd.AddCommand(exportCmd)
}

// resolveExportPath turns --output into a concrete file path: empty means
// ./errata-export-<date>.md, an existing directory gets the default name
// inside it, anything else is a file whose parent directory must exist.
func resolveExportPath(output string, now time.Time) (string, error) {
	name := "errata-export-" + now.Format("20060102") + ".md"
	if output == "" {
		return name, nil
	}
	if strings.HasSuffix(output, "/") {
		return "", fmt.Errorf("no such directory: %s", output)
	}
	if fi, err := os.Stat(output); err == nil {
		if fi.IsDir() {
			return filepath.Join(output, name), nil
		}
		return output, nil
	}
	parent := filepath.Dir(output)
	if _, err := os.Stat(parent); err != nil {
		return "", fmt.Errorf("no such directory: %s", parent)
	}
	return output, nil
}

// renderExport renders the library as plain Markdown (no ANSI): a header
// with export time and totals, then errors grouped by project, oldest
// first inside each group.
func renderExport(items []store.Error, now time.Time) string {
	groups := map[string][]store.Error{}
	for _, e := range items {
		dir := e.ProjectDir
		if dir == "" {
			dir = "(unknown project)"
		}
		groups[dir] = append(groups[dir], e)
	}
	dirs := make([]string, 0, len(groups))
	for dir := range groups {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	resolved := 0
	for _, e := range items {
		if e.Solution != "" {
			resolved++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# errata error library\n\nexported: %s\nerrors: %d total, %d with a solution\n",
		now.Format("2006-01-02 15:04:05"), len(items), resolved)
	for _, dir := range dirs {
		fmt.Fprintf(&b, "\n## %s\n", dir)
		es := groups[dir]
		sort.Slice(es, func(i, j int) bool { return es[i].FirstSeen.Before(es[j].FirstSeen) })
		for _, e := range es {
			fmt.Fprintf(&b, "\n### #%d %s\n\n", e.ID, e.Signature)
			fmt.Fprintf(&b, "- language: %s\n", orDash(e.Language))
			fmt.Fprintf(&b, "- seen: %d times (first %s, last %s)\n",
				e.Count, e.FirstSeen.Format("2006-01-02 15:04:05"), e.LastSeen.Format("2006-01-02 15:04:05"))
			if e.Solution != "" {
				fmt.Fprintf(&b, "- solution: %s\n", e.Solution)
			} else {
				fmt.Fprintf(&b, "- solution: *(pending)*\n")
			}
		}
	}
	return b.String()
}
