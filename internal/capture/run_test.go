package capture

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunCapturesStderrAndExitCode(t *testing.T) {
	res, err := Run([]string{"bash", "-c", "echo out; echo boom >&2; exit 3"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 3 {
		t.Fatalf("exit = %d, want 3", res.ExitCode)
	}
	if !strings.Contains(string(res.Stderr), "boom") {
		t.Fatalf("stderr = %q, want it to contain %q", res.Stderr, "boom")
	}
	if strings.Contains(string(res.Stderr), "out") {
		t.Fatalf("stdout leaked into stderr recording: %q", res.Stderr)
	}
}

func TestRunSuccess(t *testing.T) {
	res, err := Run([]string{"true"})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestRunCommandNotFound(t *testing.T) {
	res, err := Run([]string{"definitely-not-a-real-command-xyz"})
	if err == nil {
		t.Fatal("want start error")
	}
	if res == nil || res.ExitCode != 127 {
		t.Fatalf("res=%+v, want exit 127", res)
	}
}

func TestScene(t *testing.T) {
	ctx := Scene([]string{"bash", "-c", "true"})
	if ctx.Dir == "" || ctx.OS == "" {
		t.Fatalf("scene incomplete: %+v", ctx)
	}
	if ctx.Runtime != "" {
		t.Fatalf("bash should have no runtime probe, got %q", ctx.Runtime)
	}
	ctx = Scene([]string{"python3", "x.py"})
	if ctx.Runtime == "" {
		t.Skip("python3 not installed; probe skipped")
	}
}

// TestRunLeavesNoGoroutines guards the Run lifecycle fix: the SIGWINCH
// relay (and any pump) must exit with the command, not linger.
func TestRunLeavesNoGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	if _, err := Run([]string{"true"}); err != nil {
		t.Fatal(err)
	}
	// The relay goroutine exits on channel close; allow a moment.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutines leaked: before=%d after=%d", before, runtime.NumGoroutine())
}
