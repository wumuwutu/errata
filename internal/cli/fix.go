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

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/store"
	"github.com/wumuwutu/errata/internal/termx"
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
		defer termx.PlainUnlessTTY(cmd.ErrOrStderr())()
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
			fmt.Fprintln(cmd.ErrOrStderr(), termx.Faint("--err-- no pending errors - nothing to fix"))
			return nil
		}

		// Pain point fix: never record blindly — always show what the
		// solution will be attached to, before asking for it.
		printFixTarget(cmd.ErrOrStderr(), target)

		// Solution draft (dev-guide §7.3): in an interactive hooked
		// session, show what was run between the error and now as
		// numbered candidates. Silent no-op otherwise.
		cfg, _ := config.Load() // defaults on error; drafts are best-effort
		var drafts []string
		if (cfg == nil || cfg.DraftEnabled) && fixMessage == "" && isTerminal(cmd.InOrStdin()) {
			drafts = solutionDrafts(target)
			printDrafts(cmd.ErrOrStderr(), drafts)
		}

		solution, err := readSolution(cmd.InOrStdin(), cmd.ErrOrStderr(), fixMessage, drafts)
		if err != nil {
			return err
		}

		if err := st.AddFix(target.ID, solution); err != nil {
			return err
		}
		fmt.Fprintln(cmd.ErrOrStderr(), termx.Faint("--err-- solution recorded for error #"+
			strconv.FormatInt(target.ID, 10)))
		if more > 0 {
			fmt.Fprintln(cmd.ErrOrStderr(), termx.Faint("--err-- "+strconv.Itoa(more)+" more pending: ")+
				termx.Cyan("err pending")+termx.Faint(" to see"))
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

// printFixTarget shows the error the solution is about to be attached to.
func printFixTarget(w io.Writer, e *store.Error) {
	printTargetSummary(w, "fixing", e)
}

// printTargetSummary renders a compact two-line block — "err: <verb> #<id>:
// <signature>" first, then the scene (directory with ~ shortening,
// last-seen time, and the command that triggered it).
func printTargetSummary(w io.Writer, verb string, e *store.Error) {
	fmt.Fprintln(w, termx.Faint(fmt.Sprintf("err: %s #%d: ", verb, e.ID))+termx.Bright(e.Signature))
	fmt.Fprintln(w, termx.Faint(fmt.Sprintf("  at %s · last seen %s · cmd: %s",
		termx.ShortenHome(orDash(e.ProjectDir)),
		e.LastSeen.Format("2006-01-02 15:04"),
		termx.Truncate(orDash(e.Command), 60))))
}

// isTerminal reports whether in is an interactive terminal (the only
// place drafts are shown and picked).
func isTerminal(in io.Reader) bool {
	f, ok := in.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// readSolution resolves the solution text: -m flag wins, then piped stdin,
// then an interactive prompt. At the prompt a bare number adopts that
// draft candidate as-is (no second edit — a wrong pick is redone with
// another err fix); anything else is the handwritten solution. Empty
// solutions are rejected.
func readSolution(in io.Reader, out io.Writer, flagValue string, drafts []string) (string, error) {
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
	s := strings.TrimSpace(line)
	if s == "" {
		return "", errors.New("empty solution")
	}
	return pickDraftSolution(s, drafts), nil
}

// pickDraftSolution interprets one interactive input line: a bare number
// adopts that draft candidate, anything else is the handwritten solution.
func pickDraftSolution(s string, drafts []string) string {
	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(drafts) {
		return drafts[n-1]
	}
	return s
}
