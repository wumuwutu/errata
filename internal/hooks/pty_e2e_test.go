package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// ptySession drives an interactive shell on a real PTY, waiting for the
// prompt marker between commands (human typing cadence) — the conditions
// under which the command-attribution bugs were reported.
type ptySession struct {
	t   *testing.T
	pt  *os.File
	cmd *exec.Cmd

	mu      sync.Mutex
	buf     []byte
	pos     int
	pumpErr error // set when the pump goroutine's Read fails
}

func startPTYSession(t *testing.T, shell, bin, tmp string) *ptySession {
	t.Helper()
	env := append(os.Environ(),
		"XDG_DATA_HOME="+filepath.Join(tmp, "data"),
		"XDG_CONFIG_HOME="+filepath.Join(tmp, "conf"),
		// Isolate the hook's session dir too — otherwise tests share
		// /tmp/errata-<uid> with the developer's real sessions.
		"XDG_RUNTIME_DIR="+filepath.Join(tmp, "run"),
		"TERM=xterm",
	)
	var cmd *exec.Cmd
	if shell == "zsh" {
		zdot := filepath.Join(tmp, "zdot")
		if err := os.MkdirAll(zdot, 0o755); err != nil {
			t.Fatal(err)
		}
		rc := "PROMPT='DVP> '\n" +
			"export PATH=\"" + filepath.Dir(bin) + ":$PATH\"\n" +
			"eval \"$(" + bin + " init zsh)\"\n"
		if err := os.WriteFile(filepath.Join(zdot, ".zshrc"), []byte(rc), 0o644); err != nil {
			t.Fatal(err)
		}
		// Ubuntu's global /etc/zsh/zshrc runs compinit, which stops for an
		// interactive y/n answer when a fpath directory is group-writable
		// (as on CI runners) — and the prompt never comes. .zshenv is read
		// before the global zshrc, so skip it there.
		if err := os.WriteFile(filepath.Join(zdot, ".zshenv"), []byte("skip_global_compinit=1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		env = append(env, "ZDOTDIR="+zdot)
		cmd = exec.Command("zsh", "-i")
	} else {
		rc := filepath.Join(tmp, "rc")
		content := "PS1='DVP> '\n" +
			"export PATH=\"" + filepath.Dir(bin) + ":$PATH\"\n" +
			"eval \"$(" + bin + " init bash)\"\n"
		if err := os.WriteFile(rc, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd = exec.Command("bash", "--rcfile", rc, "-i")
	}
	cmd.Env = env

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatalf("start pty: %v", err)
	}
	s := &ptySession{t: t, pt: f, cmd: cmd}
	// A pump goroutine owns all PTY reads: os.File.Read on a PTY master is
	// a blocking syscall that can outlive SetReadDeadline (deadlines are a
	// no-op on non-polled files), which once turned a missing prompt into a
	// 10-minute hang on CI. With the pump, readUntil only ever polls the
	// buffer and its deadline is real.
	go func() {
		chunk := make([]byte, 4096)
		for {
			n, err := f.Read(chunk)
			if n > 0 {
				s.mu.Lock()
				s.buf = append(s.buf, chunk[:n]...)
				s.mu.Unlock()
			}
			if err != nil {
				s.mu.Lock()
				s.pumpErr = err
				s.mu.Unlock()
				return
			}
		}
	}()
	if !s.readUntil("DVP> ", 20*time.Second) {
		t.Fatalf("%s: shell never showed a prompt; transcript:\n%s", shell, s.transcript())
	}
	return s
}

// readUntil consumes PTY output until marker appears (after previously
// consumed output) or the deadline passes.
func (s *ptySession) readUntil(marker string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		i := strings.Index(string(s.buf[s.pos:]), marker)
		if i >= 0 {
			s.pos += i + len(marker)
		}
		s.mu.Unlock()
		if i >= 0 {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func (s *ptySession) send(line string) {
	s.t.Helper()
	if _, err := s.pt.Write([]byte(line + "\n")); err != nil {
		s.t.Fatalf("write %q: %v", line, err)
	}
	if !s.readUntil("DVP> ", 20*time.Second) {
		s.t.Fatalf("no prompt after %q; transcript:\n%s", line, s.transcript())
	}
}

// vim opens the file in vim and quits without saving (:q!). vim exits 0.
func (s *ptySession) vim(file string) {
	s.t.Helper()
	if _, err := s.pt.Write([]byte("vim " + file + "\n")); err != nil {
		s.t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond) // let vim take over the terminal
	if _, err := s.pt.Write([]byte(":q!\r")); err != nil {
		s.t.Fatal(err)
	}
	if !s.readUntil("DVP> ", 20*time.Second) {
		s.t.Fatalf("no prompt after vim %s", file)
	}
}

func (s *ptySession) close() {
	_, _ = s.pt.Write([]byte("exit\n"))
	time.Sleep(300 * time.Millisecond)
	_ = s.pt.Close() // SIGHUP for anything still alive (tee)
	_ = s.cmd.Wait()
}

func (s *ptySession) transcript() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pumpErr != nil {
		return fmt.Sprintf("%s\n[pump died: %v]", s.buf, s.pumpErr)
	}
	return string(s.buf)
}

// TestHookAttributionPTY reproduces the exact user-reported scenario in a
// hooked interactive shell on a real PTY:
//
//	python fails -> ls -> python fails -> vim -> vim (must stay quiet)
//	-> python succeeds with a DIFFERENT target (must stay quiet too)
//	-> the same script succeeds (must nudge, naming the SECOND error)
//
// and asserts every recorded error is attributed to the command that
// actually triggered it.
//
// Since v0.1.12 the success path is gated by a 5-minute window after the
// last failure this session saw: every success inside the window spawns
// one hook-event (vim saving the file must NOT eat the fixed re-run's
// nudge), successes in failure-free sessions spawn none.
func TestHookAttributionPTY(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
	if _, err := exec.LookPath("vim"); err != nil {
		t.Skip("vim not installed")
	}
	bin := buildErr(t)

	for _, shell := range []string{"bash", "zsh"} {
		shell := shell
		if _, err := exec.LookPath(shell); err != nil {
			t.Logf("%s not installed, skipping", shell)
			continue
		}
		t.Run(shell, func(t *testing.T) {
			tmp := t.TempDir()
			failA := filepath.Join(tmp, "failA.py")
			failB := filepath.Join(tmp, "failB.py")
			if err := os.WriteFile(failA, []byte("x = None\nx.foo\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			// failB fails until the FIXED env var is set, so the scenario can
			// end with a genuinely successful re-run of the same script.
			guardedB := "import os\nif not os.environ.get('FIXED'):\n    import no_such_module_zzz\n"
			if err := os.WriteFile(failB, []byte(guardedB), 0o644); err != nil {
				t.Fatal(err)
			}

			s := startPTYSession(t, shell, bin, tmp)
			s.send("python3 " + failA) // -> AttributeError, record #1
			s.send("ls")
			s.send("python3 " + failB)         // -> ModuleNotFoundError, record #2
			s.vim(failA)                       // exits 0: different program, no nudge
			s.vim(failB)                       // exits 0: same here
			s.send("python3 -c 'print(1)'")    // same program, other target: no nudge
			s.send("FIXED=1 python3 " + failB) // same program AND script: nudge #2
			// Ctrl-C at an empty prompt must not record anything against a
			// stale command line (ec=130 reuse of $?).
			//
			// Delivered as a real PTY ^C byte. A ^C that lands while the
			// shell is still redrawing its prompt can be eaten, so settle
			// and retry — extra ^Cs at an empty prompt are no-ops. (This
			// also guards the hook's tee shielding: an unshielded tee
			// died to the SIGINT group broadcast and took bash 5.2 down
			// with it via SIGPIPE.)
			gotPrompt := false
			for range 5 {
				time.Sleep(300 * time.Millisecond)
				if _, err := s.pt.Write([]byte{0x03}); err != nil {
					t.Fatal(err)
				}
				if s.readUntil("DVP> ", 5*time.Second) {
					gotPrompt = true
					break
				}
			}
			if !gotPrompt {
				t.Fatalf("no prompt after Ctrl-C; transcript:\n%s", s.transcript())
			}
			s.send("echo E2E-DONE")
			s.close()

			env := append(os.Environ(),
				"XDG_DATA_HOME="+filepath.Join(tmp, "data"),
				"XDG_CONFIG_HOME="+filepath.Join(tmp, "conf"))
			show := func(id string) string {
				c := exec.Command(bin, "show", id)
				c.Env = env
				out, _ := c.CombinedOutput()
				return string(out)
			}

			r1, r2 := show("1"), show("2")
			if !strings.Contains(r1, "AttributeError") || !strings.Contains(r1, "command:") || !strings.Contains(r1, "python3 "+failA) {
				t.Errorf("record #1 wrong:\n%s", r1)
			}
			if !strings.Contains(r2, "ModuleNotFoundError") || !strings.Contains(r2, "python3 "+failB) {
				t.Errorf("record #2 wrong:\n%s", r2)
			}
			if out := show("3"); !strings.Contains(out, "not found") {
				t.Errorf("there must be exactly 2 records; show 3:\n%s", out)
			}
			for _, r := range []string{r1, r2} {
				if strings.Contains(r, "command:") &&
					(strings.Contains(r, "vim ") || strings.Contains(r, "command:      ls")) {
					t.Errorf("command attributed to the wrong command:\n%s", r)
				}
			}

			tr := s.transcript()
			if n := strings.Count(tr, "looks fixed"); n != 1 {
				t.Errorf("looks-fixed nudge count = %d, want 1\ntranscript:\n%s", n, tr)
			}
			// The nudge must name the SECOND error (the latest pending) by
			// number and keep its two-line shape.
			for _, ln := range strings.Split(tr, "\n") {
				if strings.Contains(ln, "looks fixed") {
					if !strings.Contains(ln, "err #2") {
						t.Errorf("nudge names the wrong error: %q", ln)
					}
					if strings.Contains(ln, "err fix") {
						t.Errorf("looks-fixed must not share a line with 'err fix': %q", ln)
					}
				}
			}
			if strings.Contains(tr, "── err") {
				t.Error("hint still uses box-drawing dashes")
			}
		})
	}
}
