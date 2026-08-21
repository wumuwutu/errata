package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/capture"
	"github.com/wumuwutu/dejavu/internal/config"
	"github.com/wumuwutu/dejavu/internal/fingerprint"
	"github.com/wumuwutu/dejavu/internal/hint"
	"github.com/wumuwutu/dejavu/internal/store"
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

	if res.ExitCode == 0 || len(res.Stderr) == 0 {
		return res.ExitCode
	}

	cfg, cfgErr := config.Load()
	if cfgErr == nil {
		if cwd, err := os.Getwd(); err == nil && cfg.IsIgnored(args[0], cwd) {
			return res.ExitCode // blacklisted: pass through, never record
		}
	}

	recordAndHint(args, res, cfg)
	return res.ExitCode
}

// recordAndHint fingerprints a failed run, stores it, and prints the
// restrained hit hint when the error (or a similar one) was seen before.
// Recording must never break the run: any storage failure is silent and
// the child's exit code is untouched.
func recordAndHint(args []string, res *capture.Result, cfg *config.Config) {
	lang, signature, fp := fingerprint.Fingerprint(string(res.Stderr))
	if signature == "" {
		return // not a recognizable Python/Node error: skip, never guess
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return
	}
	defer st.Close() //nolint:errcheck // best-effort; process exits right after

	scene := capture.Scene(args)
	rec := &store.Error{
		Fingerprint: fp,
		Signature:   signature,
		RawSample:   fingerprint.StripANSI(string(res.Stderr)),
		Language:    lang,
		Command:     scene.Command,
		ProjectDir:  scene.Dir,
		GitCommit:   scene.GitCommit,
		Runtime:     scene.Runtime,
		OS:          scene.OS,
	}

	// Match BEFORE upserting so the hint reflects prior knowledge.
	existing, err := st.FindByFingerprint(fp)
	if err != nil {
		return
	}

	var hit *store.Error
	similar := false
	if existing != nil {
		hit = existing
		hit.Count++ // this occurrence included ("第N次")
	} else {
		if sim, _, serr := st.FindSimilar(fp, fingerprint.SimilarityThreshold); serr == nil && sim != nil {
			hit, similar = sim, true
		}
	}

	if _, _, err := st.UpsertError(rec); err != nil {
		return
	}

	if hit != nil && cfg != nil && cfg.HintEnabled {
		hint.PrintToStderr(hit, similar)
	}
}
