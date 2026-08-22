package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/hooks"
)

var uninstallPurge bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the shell hooks and (optionally) all data",
	Long: "uninstall removes the errata hook block from your shell rc files\n" +
		"(exactly what err init --write added), then asks whether to delete the\n" +
		"data directory (default: keep). It cannot delete the binary itself —\n" +
		"the last step is yours (uninstall red line, dev-guide §9).",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		// 1. rc files: precise removal, report every file touched.
		removedAny := false
		for _, shell := range hooks.Supported {
			rc, removed, err := hooks.RemoveRC(shell)
			if err != nil {
				return err
			}
			if removed {
				fmt.Fprintf(out, "removed hook from %s\n", rc)
				removedAny = true
			}
		}
		if !removedAny {
			fmt.Fprintln(out, "no hook found in ~/.zshrc / ~/.bashrc")
		}

		// 2. Data: kept by default; --purge deletes without asking.
		dataDir, err := config.DataDir()
		if err == nil {
			if uninstallPurge || confirmPurge(cmd.InOrStdin(), out, dataDir) {
				if err := os.RemoveAll(dataDir); err != nil {
					return err
				}
				fmt.Fprintf(out, "deleted %s\n", dataDir)
			} else {
				fmt.Fprintf(out, "kept %s (delete it manually if you change your mind)\n", dataDir)
			}
		}

		// 3. The binary: ours to point at, the user's to remove.
		if self, err := os.Executable(); err == nil {
			fmt.Fprintf(out, "\ndone. remove the binary yourself:\n  rm %s\n", self)
		}
		return nil
	},
}

func init() {
	uninstallCmd.Flags().BoolVar(&uninstallPurge, "purge", false, "delete the data directory without asking")
	rootCmd.AddCommand(uninstallCmd)
}

// confirmPurge asks before deleting data; without a terminal the safe
// default is "keep".
func confirmPurge(in io.Reader, out io.Writer, dataDir string) bool {
	f, isFile := in.(*os.File)
	if !isFile || !term.IsTerminal(int(f.Fd())) {
		return false
	}
	fmt.Fprintf(out, "delete data directory %s? [y/N] ", dataDir)
	line, _ := bufio.NewReader(in).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "y")
}
