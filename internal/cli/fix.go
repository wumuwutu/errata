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
	Short: "Record the solution for an error (default: pick from pending)",
	Long: "fix attaches your solution to an error and marks it resolved.\n" +
		"With no argument it shows the pending errors and lets you pick one.\n" +
		"Pass the solution inline with -m, pipe it on stdin, or type it at the prompt.\n" +
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

		target, err := resolveFixTarget(st, args, cmd.InOrStdin(), cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		if target == nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "err: no pending errors — nothing to fix")
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
		return nil
	},
}

func init() {
	fixCmd.Flags().StringVarP(&fixMessage, "message", "m", "", "solution text (non-interactive)")
	rootCmd.AddCommand(fixCmd)
}

// resolveFixTarget picks the error to fix: the id argument if given, the
// single pending error, or an interactive numbered choice among several.
// Returns (nil, nil) when there is nothing pending.
func resolveFixTarget(st *store.Store, args []string, in io.Reader, out io.Writer) (*store.Error, error) {
	if len(args) == 1 {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q", args[0])
		}
		e, err := st.Get(id)
		if err != nil {
			return nil, err
		}
		if e == nil {
			return nil, fmt.Errorf("error #%d not found", id)
		}
		return e, nil
	}

	items, err := st.ListPending()
	if err != nil {
		return nil, err
	}
	switch len(items) {
	case 0:
		return nil, nil
	case 1:
		return st.Get(items[0].ErrorID)
	}

	// Several pending: offer a numbered choice. Without a terminal there
	// is no one to ask — point at the explicit form instead.
	f, isFile := in.(*os.File)
	if !isFile || !term.IsTerminal(int(f.Fd())) {
		return nil, fmt.Errorf("%d pending errors — pick one with: err fix <id> (see err pending)", len(items))
	}
	fmt.Fprintln(out, "pending errors:")
	for i, it := range items {
		fmt.Fprintf(out, "  %d) [#%d] %s — %s, seen %d times, last %s\n",
			i+1, it.ErrorID, it.Signature, it.Language, it.Count,
			it.LastSeen.Format("2006-01-02 15:04"))
	}
	fmt.Fprint(out, "fix which? [1]: ")
	line, _ := bufio.NewReader(in).ReadString('\n')
	idx, ok := parseChoice(line, len(items))
	if !ok {
		return nil, errors.New("aborted")
	}
	return st.Get(items[idx].ErrorID)
}

// parseChoice parses a "1..n" menu answer; empty input means 1.
func parseChoice(input string, n int) (idx int, ok bool) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, true
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 || v > n {
		return 0, false
	}
	return v - 1, true
}

// printFixTarget shows the error the solution is about to be attached to.
func printFixTarget(w io.Writer, e *store.Error) {
	fmt.Fprintf(w, "── fixing error #%d ──\n", e.ID)
	fmt.Fprintf(w, "  signature: %s\n", e.Signature)
	fmt.Fprintf(w, "  seen:      %d times (last %s)\n", e.Count, e.LastSeen.Format("2006-01-02 15:04"))
	fmt.Fprintf(w, "  directory: %s\n", orDash(e.ProjectDir))
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
