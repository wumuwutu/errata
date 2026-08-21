// Package capture runs commands under a pseudo-terminal, passing
// stdin/stdout/stderr through untouched while recording stderr off to the
// side, and collects the scene (command, directory, git, runtime, OS).
package capture

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// Result is the outcome of a wrapped command run.
type Result struct {
	// ExitCode is the child's exit code; err run must exit with exactly this.
	ExitCode int
	// Stderr is the byte-exact stderr the child produced (tee'd, not intercepted).
	Stderr []byte
}

// lockedBuffer is an io.Writer safe for concurrent use.
type lockedBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *lockedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, len(b.buf))
	copy(out, b.buf)
	return out
}

// Run executes args under a PTY:
//
//   - stdout is attached to the PTY, so colored output behaves normally.
//   - stdin is attached to the PTY when it is a terminal (interactive
//     programs like vim/top/REPLs work, Ctrl-C reaches the child via the
//     controlling tty); when stdin is a pipe or file it is passed through
//     directly, so EOF propagates and pipelines behave as usual.
//   - stderr is streamed through a pipe and tee'd to the real stderr:
//     the user sees every byte as it arrives (no interception, no delay)
//     while a copy lands in the recording buffer.
//   - the parent terminal is put in raw mode for the duration, and window
//     resizes are relayed to the PTY.
//
// Run returns only after the child has exited; its exit code is preserved
// exactly. A failure to start the child (e.g. command not found) is
// reported as an error and exit code 127, mirroring shell behavior.
func Run(args []string) (*Result, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = os.Environ()

	stdinIsTTY := term.IsTerminal(int(os.Stdin.Fd()))

	ptmx, tty, err := pty.Open()
	if err != nil {
		return nil, err
	}

	cmd.Stdout = tty
	if stdinIsTTY {
		cmd.Stdin = tty
		// New session with the PTY as controlling terminal (same as
		// pty.Start, but stderr stays on its own pipe for recording).
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	} else {
		cmd.Stdin = os.Stdin
	}

	closeTTY := func() {
		if tty != nil {
			tty.Close()
			tty = nil
		}
	}
	defer closeTTY()

	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		ptmx.Close()
		return nil, err
	}
	cmd.Stderr = stderrW

	var stderrBuf lockedBuffer
	teeDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.MultiWriter(os.Stderr, &stderrBuf), stderrR)
		close(teeDone)
	}()

	// Track the real terminal's size and follow resizes. When stdin is not
	// a terminal (piped), inherit the size from stdout/stderr instead.
	sizeSrc := os.Stdin
	if !stdinIsTTY {
		if term.IsTerminal(int(os.Stdout.Fd())) {
			sizeSrc = os.Stdout
		} else if term.IsTerminal(int(os.Stderr.Fd())) {
			sizeSrc = os.Stderr
		}
	}
	_ = pty.InheritSize(sizeSrc, ptmx)
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	// signal.Stop stops future sends; closing the channel then lets the
	// relay goroutine exit with the command instead of lingering forever.
	defer func() { signal.Stop(winch); close(winch) }()
	go func() {
		for range winch {
			_ = pty.InheritSize(sizeSrc, ptmx)
		}
	}()

	// Raw mode so interactive children receive keystrokes unbuffered.
	if stdinIsTTY {
		if old, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
			defer term.Restore(int(os.Stdin.Fd()), old) //nolint:errcheck
		}
	}

	if err := cmd.Start(); err != nil {
		ptmx.Close()
		stderrW.Close()
		<-teeDone
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return &Result{ExitCode: 127}, err
		}
		return nil, err
	}
	// The child holds its own dup of the slave side; the parent must close
	// its copy, otherwise reading the master never sees EOF/EIO when the
	// child exits and the output pump below would hang forever.
	closeTTY()

	if stdinIsTTY {
		// Put stdin in non-blocking mode so the pump can notice the done
		// channel even when no input arrives — a blocking Read on a
		// terminal cannot be interrupted portably. Restored on return
		// (the flag lives on the open file description shared with the
		// invoking shell, so failing to restore would leak into it).
		fd := int(os.Stdin.Fd())
		if err := syscall.SetNonblock(fd, true); err == nil {
			defer syscall.SetNonblock(fd, false) //nolint:errcheck
			pumpDone := make(chan struct{})
			defer close(pumpDone)
			go pumpStdin(ptmx, pumpDone)
		} else {
			// Fall back to a plain pump; it exits with the process.
			go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
		}
	}
	outDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(os.Stdout, ptmx) // PTY -> stdout
		close(outDone)
	}()

	waitErr := cmd.Wait()

	// Drain and close in order: child is gone, flush remaining output.
	stderrW.Close()
	<-teeDone
	ptmx.Close()
	<-outDone

	res := &Result{ExitCode: 0, Stderr: stderrBuf.Bytes()}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else {
			return res, waitErr
		}
	}
	return res, nil
}

// pumpStdin copies keystrokes into the PTY until done is closed. It relies
// on stdin being non-blocking (set by Run): EAGAIN means "no input right
// now", so the pump can poll done between reads instead of blocking on a
// terminal Read forever.
func pumpStdin(ptmx *os.File, done <-chan struct{}) {
	buf := make([]byte, 1024)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if _, werr := ptmx.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, syscall.EAGAIN) {
			select {
			case <-done:
				return
			case <-time.After(5 * time.Millisecond):
			}
			continue
		}
		return
	}
}
