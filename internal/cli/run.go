package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/errata/internal/capture"
	"github.com/wumuwutu/errata/internal/config"
)

var runCmd = &cobra.Command{
	Use:   "run <cmd> [args...]",
	Short: "Run a command under a PTY, capturing any error",
	Long: "Run executes the command in a pseudo-terminal. stdin/stdout/stderr are passed\n" +
		"through untouched; stderr is recorded on the side. If the command fails with\n" +
		"non-empty stderr, the error is fingerprinted, stored, and matched against\n" +
		"your history. err run always exits with the wrapped command's exit code.",
	Example:            "  err run python train.py\n  err run node app.js",
	Args:               cobra.ArbitraryArgs,
	DisableFlagParsing: true, // pass all flags through to the wrapped command
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return errors.New("usage: err run <cmd> [args...]")
		}
		if args[0] == "-h" || args[0] == "--help" {
			return cmd.Help()
		}
		os.Exit(runWrapped(args))
		return nil // unreachable
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}

// runWrapped executes the command and, unless ignored, records failures.
// It returns the exit code err must terminate with — always the child's.
func runWrapped(args []string) int {
	res, err := capture.Run(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "err: %v\n", err)
		if res != nil {
			return res.ExitCode // 127, shell-style "command not found"
		}
		return 1
	}

	if res.ExitCode == 0 {
		// A successful wrapped command right after a failure of the SAME
		// program is probably the fix — the DETECTED_SUCCESS nudge.
		solvedHint(cwd(), strings.Join(args, " "), os.Stderr)
		return 0
	}
	if len(res.Stderr) == 0 {
		return res.ExitCode
	}

	cfg, _ := config.Load() // config problems must never affect the run
	recordFailure(strings.Join(args, " "), cwd(), res.Stderr, cfg, os.Stderr)
	return res.ExitCode
}

func cwd() string {
	dir, _ := os.Getwd()
	return dir
}
