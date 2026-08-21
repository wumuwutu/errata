package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/hooks"
)

var initWrite bool

var initCmd = &cobra.Command{
	Use:   "init <shell>",
	Short: "Print the shell hook script (zsh, bash)",
	Long: "init prints the shell hook that captures failing commands transparently.\n" +
		"Add it to your shell's rc file:\n\n" +
		"  zsh:  echo 'eval \"$(err init zsh)\"' >> ~/.zshrc\n" +
		"  bash: echo 'eval \"$(err init bash)\"' >> ~/.bashrc\n\n" +
		"Or let err do it with --write. fish and other shells are not supported.",
	Example: "  err init zsh\n  err init bash --write",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := strings.ToLower(args[0])
		script, ok := hooks.Script(shell)
		if !ok {
			// Graceful rejection: message on stderr, nothing on stdout
			// (so eval "$(err init fish)" is a no-op), exit 0.
			fmt.Fprintf(os.Stderr,
				"err: no shell hook for %q (supported: %s). Everything else works: use `err run <cmd>`.\n",
				args[0], strings.Join(hooks.Supported, ", "))
			return nil
		}

		if !initWrite {
			fmt.Print(script)
			return nil
		}

		rcPath, already, err := hooks.WriteRC(shell)
		if err != nil {
			return err
		}
		if already {
			fmt.Fprintf(os.Stderr, "err: %s already contains the hook line — nothing changed\n", rcPath)
			return nil
		}
		fmt.Fprintf(os.Stderr, "err: appended to %s:\n", rcPath)
		fmt.Fprintf(os.Stderr, "  # dejavu shell hook\n  %s\n", hooks.EvalLine(shell))
		fmt.Fprintf(os.Stderr, "restart your shell or run: source %s\n", rcPath)
		return nil
	},
}

func init() {
	initCmd.Flags().BoolVar(&initWrite, "write", false, "append the eval line to the shell's rc file")
	rootCmd.AddCommand(initCmd)
}
