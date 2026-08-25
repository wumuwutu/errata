package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/fingerprint"
)

var watchCmd = &cobra.Command{
	Use:   "watch [file]",
	Short: "Follow a log stream and capture errors as they appear",
	Long: "Watch reads a stream — stdin (a pipe) or a file tailed from its current end\n" +
		"(history is not replayed) — and records every recognizable error in it,\n" +
		"printing the usual hit hint when one was seen before.\n\n" +
		"Unlike the shell hook, watch sees bare text without an exit code: anything\n" +
		"that fingerprints as an error is recorded. That is the point of watch —\n" +
		"docker logs, build logs, CI output — but it also means watch is more\n" +
		"eager than the hook path. The watched stream itself is not echoed.",
	Example: "  docker logs -f myapp 2>&1 | err watch\n  tail -f build.log | err watch\n  err watch /var/log/myapp.log",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load() // config problems must never affect watching
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		var n int
		var err error
		if len(args) == 1 {
			n, err = watchFile(ctx, args[0], cwd(), cfg, cmd.OutOrStdout())
		} else {
			n, err = watchStream(ctx, os.Stdin, "stdin", cwd(), cfg, cmd.OutOrStdout())
		}
		fmt.Fprintf(cmd.OutOrStdout(), "--err-- watch: stopped, %d error(s) captured\n", n)
		return err
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)
}

// blockStart matches lines that begin a fresh error report. Log streams
// rarely put blank lines between tracebacks, so without this a whole log
// would merge into one block and only part of it would be fingerprinted.
// The list is deliberately narrow: a generic "Error:" start would split a
// Python traceback off its exception line (the extractor needs them
// together), so only shapes that always START a report qualify.
var blockStart = regexp.MustCompile(`^(?:Traceback \(most recent call last\):|Exception in thread "|panic:|[\w./\\-]+\.(?:go|c|cc|cpp|cxx|h|hh|hpp):\d+(?::\d+)?:)`)

// maxBlockLines bounds one block: a runaway stack gets cut rather than
// buffering forever. The remainder flows into the next block.
const maxBlockLines = 200

// blockSplitter groups lines into error blocks: a blank line ends a block,
// and a blockStart line while a block is open closes it first.
type blockSplitter struct {
	lines []string
}

// feed adds one line and returns a finished block, if any.
func (s *blockSplitter) feed(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return s.flush()
	}
	if len(s.lines) > 0 && (blockStart.MatchString(trimmed) || len(s.lines) >= maxBlockLines) {
		block, _ := s.flush()
		s.lines = append(s.lines, line)
		return block, true
	}
	s.lines = append(s.lines, line)
	return "", false
}

func (s *blockSplitter) flush() (string, bool) {
	if len(s.lines) == 0 {
		return "", false
	}
	block := strings.Join(s.lines, "\n")
	s.lines = s.lines[:0]
	return block, true
}

// watcher turns a byte stream into error records: bytes are split into
// lines, lines into blocks, and every block that fingerprints as an error
// is recorded through the same pipeline as err run / the shell hook.
type watcher struct {
	source   string
	dir      string
	cfg      *config.Config
	out      io.Writer
	split    blockSplitter
	pending  string // partial line not yet newline-terminated
	lastData time.Time
	captured int
}

func (w *watcher) feed(chunk string) {
	w.lastData = time.Now()
	w.pending += chunk
	for {
		i := strings.IndexByte(w.pending, '\n')
		if i < 0 {
			return
		}
		line := w.pending[:i]
		w.pending = w.pending[i+1:]
		if block, ok := w.split.feed(line); ok {
			w.record(block)
		}
	}
}

// idleFlush closes the open block when the stream has gone quiet: in
// follow mode there is no EOF to trigger finish(), so the last error of a
// burst would otherwise wait forever for a terminator that never comes.
// A partial line is flushed too — a writer pausing mid-line for half a
// second is rarer than a writer ending a block without a blank line.
func (w *watcher) idleFlush() {
	if w.pending != "" {
		w.split.feed(w.pending)
		w.pending = ""
	}
	if block, ok := w.split.flush(); ok {
		w.record(block)
	}
}

// idleAfter is how quiet a followed stream must be before an open block is
// flushed. Generous for log bursts, short enough to feel live.
const idleAfter = 500 * time.Millisecond

// finish flushes whatever a stream left open at EOF.
func (w *watcher) finish() {
	w.idleFlush()
}

func (w *watcher) record(block string) {
	// A stream has no exit code: text that fingerprints as an error IS an
	// error (watch semantics, unlike the hook path which requires a
	// non-zero exit). Anything unrecognized is skipped silently.
	if _, sig, _ := fingerprint.Fingerprint(block); sig == "" {
		return
	}
	recordFailure("watch: "+w.source, w.dir, []byte(block), w.cfg, w.out)
	w.captured++
}

// watchStream consumes r until EOF — a pipe stays open only as long as its
// writer lives, so `echo ... | err watch` ends on its own — or until ctx
// is cancelled.
func watchStream(ctx context.Context, r io.Reader, source, dir string, cfg *config.Config, out io.Writer) (int, error) {
	if f, ok := r.(*os.File); ok {
		// A blocked read won't notice Ctrl-C on its own; closing the file
		// fails the read and lets us exit.
		go func() { <-ctx.Done(); f.Close() }() //nolint:errcheck
	}
	w := &watcher{source: source, dir: dir, cfg: cfg, out: out}
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			w.feed(string(buf[:n]))
		}
		if err != nil {
			w.finish()
			if err == io.EOF || ctx.Err() != nil {
				return w.captured, nil
			}
			return w.captured, err
		}
	}
}

// watchFile tails path from its current end (history is not replayed) and
// consumes appends until ctx is cancelled. Polling beats pulling in a
// filesystem-notification dependency for this.
func watchFile(ctx context.Context, path, dir string, cfg *config.Config, out io.Writer) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return 0, err
	}

	w := &watcher{source: path, dir: dir, cfg: cfg, out: out}
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return w.captured, nil
		default:
		}
		n, err := f.Read(buf)
		if n > 0 {
			w.feed(string(buf[:n]))
		}
		if err != nil { // io.EOF: nothing appended yet
			if !w.lastData.IsZero() && time.Since(w.lastData) > idleAfter {
				w.idleFlush()
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}
