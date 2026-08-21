package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/hooks"
	"github.com/wumuwutu/dejavu/internal/store"
)

// hookBudget is the prompt-path performance red line (dev-guide §9).
const hookBudget = 50 * time.Millisecond

type checkStatus int

const (
	checkOK checkStatus = iota
	checkWarn
	checkFail
)

type checkResult struct {
	status checkStatus
	name   string
	detail string
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Self-check: database, hook installation, prompt latency budget",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "err doctor — dejavu v%s\n\n", Version)

		var results []checkResult
		results = append(results, checkDatabase())
		results = append(results, checkConfig())
		results = append(results, checkHook())
		results = append(results, checkHookLatency())
		results = append(results, checkDataDir())

		failed := false
		for _, r := range results {
			mark := "ok  "
			switch r.status {
			case checkWarn:
				mark = "warn"
			case checkFail:
				mark = "FAIL"
				failed = true
			}
			fmt.Fprintf(out, "%s  %-12s %s\n", mark, r.name, r.detail)
		}
		if failed {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func checkDatabase() checkResult {
	dbPath, err := config.DBPath()
	if err != nil {
		return checkResult{checkFail, "database", err.Error()}
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return checkResult{checkFail, "database", fmt.Sprintf("%s: %v", dbPath, err)}
	}
	defer st.Close()
	v, err := st.SchemaVersion()
	if err != nil {
		return checkResult{checkFail, "database", err.Error()}
	}
	if v != store.LatestSchemaVersion() {
		return checkResult{checkWarn, "database",
			fmt.Sprintf("schema v%d, this binary wants v%d", v, store.LatestSchemaVersion())}
	}
	return checkResult{checkOK, "database", fmt.Sprintf("%s (schema v%d, writable)", dbPath, v)}
}

func checkConfig() checkResult {
	dir, err := config.ConfigDir()
	if err != nil {
		return checkResult{checkWarn, "config", err.Error()}
	}
	p := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return checkResult{checkOK, "config", p + " (absent, defaults in use)"}
	}
	if _, err := config.Load(); err != nil {
		return checkResult{checkFail, "config", fmt.Sprintf("%s: %v", p, err)}
	}
	return checkResult{checkOK, "config", p}
}

// checkHook verifies the hook line is in the current shell's rc and that
// this shell actually loaded it (DEJAVU_HOOK marker, exported by the hook).
func checkHook() checkResult {
	shell := filepath.Base(os.Getenv("SHELL"))
	if _, ok := hooks.Script(shell); !ok {
		return checkResult{checkWarn, "hook", fmt.Sprintf("shell %q has no hook (supported: zsh, bash)", shell)}
	}
	rc, err := hooks.RCFile(shell)
	if err != nil {
		return checkResult{checkFail, "hook", err.Error()}
	}
	data, _ := os.ReadFile(rc)
	installed := strings.Contains(string(data), hooks.EvalLine(shell))
	loaded := os.Getenv("DEJAVU_HOOK") != ""
	switch {
	case installed && loaded:
		return checkResult{checkOK, "hook", fmt.Sprintf("%s hook installed in %s and active", shell, rc)}
	case installed:
		return checkResult{checkWarn, "hook", fmt.Sprintf("installed in %s but not active in this shell (restart it?)", rc)}
	default:
		return checkResult{checkWarn, "hook", fmt.Sprintf("not installed — run: err init %s --write", shell)}
	}
}

// checkHookLatency measures the real prompt-path cost: one hook-event
// invocation against the actual database, compared to the 50ms red line.
func checkHookLatency() checkResult {
	self, err := os.Executable()
	if err != nil {
		return checkResult{checkWarn, "hook latency", err.Error()}
	}
	cwd, _ := os.Getwd()
	start := time.Now()
	c := exec.Command(self, "hook-event", "--exit-code", "0", "--cwd", cwd)
	c.Stdout = nil
	c.Stderr = nil
	if err := c.Run(); err != nil {
		return checkResult{checkWarn, "hook latency", err.Error()}
	}
	d := time.Since(start)
	status := checkOK
	if d > hookBudget {
		status = checkFail
	}
	return checkResult{status, "hook latency", fmt.Sprintf("hook-event success path: %dms (budget %dms)", d.Milliseconds(), hookBudget.Milliseconds())}
}

func checkDataDir() checkResult {
	dir, err := config.DataDir()
	if err != nil {
		return checkResult{checkWarn, "data dir", err.Error()}
	}
	var total int64
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error { //nolint:errcheck
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return checkResult{checkOK, "data dir", fmt.Sprintf("%s (%s)", dir, humanBytes(total))}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
