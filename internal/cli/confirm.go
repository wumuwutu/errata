package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// confirmDestructive gates a destructive command behind a typed answer on
// a real terminal; without one it refuses (callers offer --yes).
func confirmDestructive(in io.Reader, out io.Writer, prompt string, requireYes bool) (bool, error) {
	f, isFile := in.(*os.File)
	tty := isFile && term.IsTerminal(int(f.Fd()))
	return confirmAnswer(in, out, prompt, requireYes, tty)
}

// confirmAnswer implements the confirmation semantics: requireYes accepts
// only the full word "yes" (clear); otherwise "y"/"yes" in any case
// confirm (delete). Anything else — including an empty line — means no.
func confirmAnswer(in io.Reader, out io.Writer, prompt string, requireYes, tty bool) (bool, error) {
	if !tty {
		return false, errors.New("refusing to proceed without confirmation (non-interactive; use --yes)")
	}
	fmt.Fprint(out, prompt)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false, err
	}
	answer := strings.TrimSpace(line)
	if requireYes {
		return answer == "yes", nil
	}
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}
