package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/hooks"
)

var hookEvent struct {
	exitCode   int
	offset     int64
	seq        int64
	stderrFile string
	cwd        string
	command    string
}

// sentinelPrefix delimits command boundaries in the session stderr buffer.
// The shell hook writes an invisible OSC escape (sentinelPrefix+seq+"\a")
// to stderr in preexec/precmd setup; because the tee pipe is FIFO, the
// sentinel lands in the buffer behind any still-unflushed bytes of earlier
// commands. The stderr delta of a failing command is therefore everything
// after ITS sentinel — late bytes from previous commands (prompt echo,
// slow tee flushes, output of `err ...` commands) can never leak in, no
// matter how far behind the tee subprocess is. Keep in sync with the
// printf lines in internal/hooks/scripts/dejavu.{bash,zsh}.
const sentinelPrefix = "\x1b]6973;dejavu;"

// hookEventCmd is the internal entry point the shell hook calls after a
// failing command. It is hidden from help and must NEVER break the prompt:
// every failure path is silent and returns success.
var hookEventCmd = &cobra.Command{
	Use:    "hook-event",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Success path (dev-guide §7.2 DETECTED_SUCCESS): the command
		// after a failure may be the fix — nudge once, cheaply.
		if hookEvent.exitCode == 0 {
			solvedHint(hookEvent.cwd, hookEvent.command, os.Stdout)
			return nil
		}
		if hookEvent.stderrFile == "" || hookEvent.command == "" {
			return nil
		}

		// Housekeeping on the error path only: drop week-old session
		// buffers so they don't pile up.
		hooks.CleanStaleSessions(filepath.Dir(hookEvent.stderrFile), time.Now()) //nolint:errcheck

		delta := readStderrDelta(hookEvent.stderrFile, hookEvent.offset, hookEvent.seq)
		if len(bytes.TrimSpace(delta)) == 0 {
			// The tee subprocess may lag behind the prompt; allow one
			// short grace re-read before concluding there was no stderr.
			time.Sleep(15 * time.Millisecond)
			delta = readStderrDelta(hookEvent.stderrFile, hookEvent.offset, hookEvent.seq)
			if len(bytes.TrimSpace(delta)) == 0 {
				return nil
			}
		}

		cfg, _ := config.Load() // missing/broken config must not break the prompt
		recordFailure(hookEvent.command, hookEvent.cwd, delta, cfg, os.Stdout)
		return nil
	},
}

func init() {
	f := hookEventCmd.Flags()
	f.IntVar(&hookEvent.exitCode, "exit-code", 0, "exit code of the failed command")
	f.Int64Var(&hookEvent.offset, "offset", 0, "byte offset into the stderr buffer at command start")
	f.Int64Var(&hookEvent.seq, "seq", 0, "command sequence number matching the hook's stderr sentinel")
	f.StringVar(&hookEvent.stderrFile, "stderr-file", "", "per-session stderr buffer written by the hook")
	f.StringVar(&hookEvent.cwd, "cwd", "", "directory the command ran in")
	f.StringVar(&hookEvent.command, "command", "", "full command line")
	rootCmd.AddCommand(hookEventCmd)
}

// readStderrDelta returns the stderr bytes belonging to the command that
// triggered this hook event: everything appended to file since offset AND
// after that command's sentinel. A positive seq requires the sentinel to
// be present; if it has not landed yet (tee lag) the result is empty —
// missing a capture is acceptable, attributing one to the wrong command
// is not (precision red line). seq == 0 means the shell runs a pre-sentinel
// hook (binary upgraded, shell not restarted): fall back to the raw
// offset slice.
func readStderrDelta(file string, offset, seq int64) []byte {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil
	}
	slice, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	if seq <= 0 {
		return slice
	}
	mark := fmt.Appendf(nil, "%s%d\a", sentinelPrefix, seq)
	i := bytes.LastIndex(slice, mark)
	if i < 0 {
		return nil
	}
	return slice[i+len(mark):]
}
