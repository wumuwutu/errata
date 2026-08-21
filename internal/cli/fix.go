package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/store"
)

var fixMessage string

var fixCmd = &cobra.Command{
	Use:   "fix [id]",
	Short: "Record the solution for an error (default: most recent pending)",
	Long: "fix attaches your solution to an error and marks it resolved.\n" +
		"With no argument it targets the latest pending error; pass an id\n" +
		"(see err pending) to record a solution for a specific error.\n" +
		"Pass the solution inline with -m, pipe it on stdin, or type it at the prompt.\n" +
		"From then on, the solution pops up automatically when the error recurs.",
	Example: "  err fix -m \"LD_LIBRARY_PATH was polluted by conda; conda deactivate and reinstall\"\n" +
		"  err fix 3 -m \"pin torch==2.1 in requirements.txt\"\n" +
		"  echo \"pip install --force-reinstall torch\" | err fix",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		solution, err := readSolution(fixMessage)
		if err != nil {
			return err
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

		var errorID int64
		var signature string
		if len(args) == 1 {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q", args[0])
			}
			e, err := st.Get(id)
			if err != nil {
				return err
			}
			if e == nil {
				return fmt.Errorf("error #%d not found", id)
			}
			errorID, signature = e.ID, e.Signature
		} else {
			p, err := st.LatestPending()
			if err != nil {
				return err
			}
			if p == nil {
				fmt.Fprintln(os.Stderr, "err: no pending errors — nothing to fix")
				return nil
			}
			errorID, signature = p.ErrorID, p.Signature
		}

		if err := st.AddFix(errorID, solution); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "err: solution recorded for error #%d (%s)\n", errorID, signature)
		return nil
	},
}

func init() {
	fixCmd.Flags().StringVarP(&fixMessage, "message", "m", "", "solution text (non-interactive)")
	rootCmd.AddCommand(fixCmd)
}

// readSolution resolves the solution text: -m flag wins, then piped stdin,
// then an interactive prompt. Empty solutions are rejected.
func readSolution(flagValue string) (string, error) {
	if s := strings.TrimSpace(flagValue); s != "" {
		return s, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, nil
		}
		return "", errors.New("empty solution (use -m \"...\" or pipe text on stdin)")
	}
	fmt.Fprint(os.Stderr, "solution> ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	if s := strings.TrimSpace(line); s != "" {
		return s, nil
	}
	return "", errors.New("empty solution")
}
