package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFixDraftPTY drives the dev-guide §7.3 flow end to end on a real
// PTY: a command fails, the user changes the environment and re-runs it
// successfully, then `err fix` shows the commands run since the error as
// numbered drafts and typing "1" adopts one as the solution.
func TestFixDraftPTY(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
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
			fail := filepath.Join(tmp, "fail.py")
			// Fails until FIXED is set, so the scenario ends with a
			// genuinely successful re-run of the same script.
			guarded := "import os\nif not os.environ.get('FIXED'):\n    import no_such_module_zzz\n"
			if err := os.WriteFile(fail, []byte(guarded), 0o644); err != nil {
				t.Fatal(err)
			}

			s := startPTYSession(t, shell, bin, tmp)
			s.send("python3 " + fail) // ModuleNotFoundError -> pending #1
			s.send("ls")              // noise: must never be a draft
			s.send("export FIXED=1")  // the actual fix: a tier-0 draft
			s.send("python3 " + fail) // now succeeds; the nudge fires

			// err fix on a real terminal shows the drafts; "1" adopts one.
			if _, err := s.pt.Write([]byte("err fix\n")); err != nil {
				t.Fatal(err)
			}
			if !s.readUntil("solution> ", 20*time.Second) {
				t.Fatalf("err fix never prompted; transcript:\n%s", s.transcript())
			}
			tr := s.transcript()
			if !strings.Contains(tr, "since the error you ran:") {
				t.Errorf("no draft block shown; transcript:\n%s", tr)
			}
			if !strings.Contains(tr, "1. export FIXED=1") {
				t.Errorf("draft list missing the export fix; transcript:\n%s", tr)
			}
			if strings.Contains(tr, "2. ls") {
				t.Errorf("noise command shown as a draft; transcript:\n%s", tr)
			}
			if _, err := s.pt.Write([]byte("1\n")); err != nil {
				t.Fatal(err)
			}
			if !s.readUntil("DVP> ", 20*time.Second) {
				t.Fatalf("no prompt after adopting the draft; transcript:\n%s", s.transcript())
			}
			s.close()

			c := exec.Command(bin, "show", "1")
			c.Env = append(os.Environ(),
				"XDG_DATA_HOME="+filepath.Join(tmp, "data"),
				"XDG_CONFIG_HOME="+filepath.Join(tmp, "conf"))
			out, _ := c.CombinedOutput()
			if !strings.Contains(string(out), "export FIXED=1") {
				t.Errorf("solution not recorded from the adopted draft:\n%s", out)
			}
		})
	}
}
