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
	Short: "Record the solution for an error (default: the latest pending one)",
	Long: "fix attaches your solution to an error and marks it resolved.\n" +
		"With no argument it targets the most recent pending error — the one you\n" +
		"probably just fixed. Pass the solution inline with -m, pipe it on stdin,\n" +
		"or type it at the prompt.\n" +
		"From then on, the solution pops up automatically when the error recurs.",
	Example: "  err fix\n" +
		"  err fix 3 -m \"pin torch==2.1 in requirements.txt\"\n" +
		"  echo \"pip install --force-reinstall torch\" | err fix 3",
	Args: cobra.MaximumNArgs(1),
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

		target, more, err := resolveFixTarget(st, args)
		if err != nil {
			return err
		}
		if target == nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "err: no pending errors - nothing to fix")
			return nil
		}

		// Pain point fix: never record blindly — always show what the
		// solution will be attached to, before asking for it.
		printFixTarget(cmd.ErrOrStderr(), target)

		solution, err := readSolution(cmd.InOrStdin(), cmd.ErrOrStderr(), fixMessage)
		if err != nil {
			return err
		}

		if err := st.AddFix(target.ID, solution); err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "err: solution recorded for error #%d (%s)\n", target.ID, target.Signature)
		if more > 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "err: %d more pending: err pending to see\n", more)
		}
		return nil
	},
}

func init() {
	fixCmd.Flags().StringVarP(&fixMessage, "message", "m", "", "solution text (non-interactive)")
	rootCmd.AddCommand(fixCmd)
}

// resolveFixTarget picks the error to fix: the id argument if given,
// otherwise the most recent pending error — "fix the problem I just had",
// no candidate list. Returns (nil, 0, nil) when there is nothing pending.
// more reports how many other pending errors remain.
func resolveFixTarget(st *store.Store, args []string) (target *store.Error, more int, err error) {
	if len(args) == 1 {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid id %q", args[0])
		}
		e, err := st.Get(id)
		if err != nil {
			return nil, 0, err
		}
		if e == nil {
			return nil, 0, fmt.Errorf("error #%d not found", id)
		}
		return e, 0, nil
	}

	items, err := st.ListPending()
	if err != nil {
		return nil, 0, err
	}
	if len(items) == 0 {
		return nil, 0, nil
	}
	// ListPending orders by detected_at DESC: items[0] is the latest.
	e, err := st.Get(items[0].ErrorID)
	if err != nil {
		return nil, 0, err
	}
	return e, len(items) - 1, nil
}

// printFixTarget shows, in one line, the error the solution is about to be
// attached to: signature, directory, last-seen time.
func printFixTarget(w io.Writer, e *store.Error) {
	fmt.Fprintf(w, "err: fixing #%d: %s (%s, last seen %s)\n",
		e.ID, e.Signature, orDash(e.ProjectDir), e.LastSeen.Format("2006-01-02 15:04"))
}

// readSolution resolves the solution text: -m flag wins, then piped stdin,
// then an interactive prompt. Empty solutions are rejected.
func readSolution(in io.Reader, out io.Writer, flagValue string) (string, error) {
	if s := strings.TrimSpace(flagValue); s != "" {
		return s, nil
	}
	if f, isFile := in.(*os.File); !isFile || !term.IsTerminal(int(f.Fd())) {
		b, err := io.ReadAll(in)
		if err != nil {
			return "", err
		}
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, nil
		}
		return "", errors.New("empty solution (use -m \"...\" or pipe text on stdin)")
	}
	fmt.Fprint(out, "solution> ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	if s := strings.TrimSpace(line); s != "" {
		return s, nil
	}
	return "", errors.New("empty solution")
}
