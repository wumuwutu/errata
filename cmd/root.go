// Package cmd wires the cobra command tree for err.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is the dejavu release version.
const Version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:     "err",
	Short:   "dejavu — a personal memory for terminal errors",
	Long:    "err (dejavu) captures failing commands, fingerprints errors, remembers your fixes,\nand hands the fix back the next time the same error shows up.",
	Version: Version,
	// The wrapper subcommand (err run) exits with the child's exit code;
	// usage noise on its errors would be misleading.
	SilenceUsage: true,
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
