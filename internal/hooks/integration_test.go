package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScriptSupported(t *testing.T) {
	for _, sh := range Supported {
		s, ok := Script(sh)
		if !ok || s == "" {
			t.Fatalf("Script(%q) missing", sh)
		}
	}
	if _, ok := Script("fish"); ok {
		t.Fatal("fish must not be supported")
	}
	if _, ok := Script("powershell"); ok {
		t.Fatal("powershell must not be supported")
	}
}

func TestWriteRCIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path, already, err := WriteRC("zsh")
	if err != nil || already {
		t.Fatalf("first WriteRC: path=%q already=%v err=%v", path, already, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), EvalLine("zsh")) {
		t.Fatalf("rc missing eval line:\n%s", data)
	}

	_, already, err = WriteRC("zsh")
	if err != nil || !already {
		t.Fatalf("second WriteRC: already=%v err=%v", already, err)
	}
	data2, _ := os.ReadFile(path)
	if len(data2) != len(data) {
		t.Fatal("second WriteRC must not append again")
	}
}

// TestHookIntegration runs the shell integration scripts against a freshly
// built err binary. Skipped with -short or when the shell is missing.
func TestHookIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := buildErr(t)

	shells := map[string]string{
		"bash": "hook-it.bash",
		"zsh":  "hook-it.zsh",
	}
	for shell, script := range shells {
		shell, script := shell, script
		if _, err := exec.LookPath(shell); err != nil {
			t.Logf("%s not installed, skipping", shell)
			continue
		}
		t.Run(shell, func(t *testing.T) {
			cmd := exec.Command("bash", repoFile(t, "scripts", script), bin)
			out, err := cmd.CombinedOutput()
			t.Logf("\n%s", out)
			if err != nil {
				t.Fatalf("%s hook integration failed: %v", shell, err)
			}
		})
	}
}

// TestHookPromptOverhead is a coarse guard for the <50ms red line: a hooked
// interactive session running 30 successful commands must stay fast.
// (Success path runs zero err processes; overhead is the DEBUG trap and
// prompt function only.)
func TestHookPromptOverhead(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	bin := buildErr(t)

	rc := filepath.Join(t.TempDir(), "rc")
	content := "export PATH=\"" + filepath.Dir(bin) + ":$PATH\"\neval \"$(\"" + bin + "\" init bash)\"\n"
	if err := os.WriteFile(rc, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdin string
	for i := 0; i < 30; i++ {
		stdin += "true\n"
	}
	start := time.Now()
	cmd := exec.Command("bash", "--rcfile", rc, "-i")
	cmd.Env = append(os.Environ(), "XDG_DATA_HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	in, _ := cmd.StdinPipe()
	go func() {
		defer in.Close()
		in.Write([]byte(stdin))
	}()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hooked session failed: %v\n%s", err, out)
	}
	d := time.Since(start)
	t.Logf("30 hooked prompts took %v (%.1fms/prompt)", d, float64(d.Milliseconds())/30)
	if d > 10*time.Second {
		t.Fatalf("hooked session too slow: %v", d)
	}
}

func buildErr(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "err")
	cmd := exec.Command("go", "build", "-o", bin, repoFile(t, "cmd", "err"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build err: %v\n%s", err, out)
	}
	return bin
}

func repoFile(t *testing.T, elems ...string) string {
	t.Helper()
	// Tests run in internal/hooks; the repo root is two levels up.
	parts := append([]string{"..", ".."}, elems...)
	return filepath.Join(parts...)
}
