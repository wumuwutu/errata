// Package cli wires the cobra command tree for err.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/store"
)

// Version is the errata release version.
const Version = "0.1.15"

var rootCmd = &cobra.Command{
	Use:     "err",
	Short:   "errata — a personal memory for terminal errors",
	Long:    "err (errata) captures failing commands, fingerprints errors, remembers your fixes,\nand hands the fix back the next time the same error shows up.",
	Version: Version,
	// The wrapper subcommand (err run) exits with the child's exit code;
	// usage noise on its errors would be misleading. Errors are printed
	// once by Execute.
	SilenceUsage:  true,
	SilenceErrors: true,
	// Lazily archive stale pending errors on startup (no background
	// process, dev-guide §7.5). Skipped on the prompt-critical hook path;
	// all failures are silent — housekeeping must never break a command.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		switch cmd.Name() {
		case "hook-event", "init", "help":
			return
		}
		archiveStalePending()
	},
}

// archiveStalePending archives pending errors older than the configured
// horizon (archive_after_days, default 30). Best-effort, silent.
func archiveStalePending() {
	cfg, err := config.Load()
	if err != nil || cfg.ArchiveAfterDays <= 0 {
		return
	}
	dbPath, err := config.DBPath()
	if err != nil {
		return
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return
	}
	defer st.Close() //nolint:errcheck
	_, _ = st.ArchiveStalePending(time.Now().AddDate(0, 0, -cfg.ArchiveAfterDays))
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "err:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}
