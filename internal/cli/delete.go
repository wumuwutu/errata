package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/store"
	"github.com/wumuwutu/errata/internal/termx"
)

var deleteYes bool

var deleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete an error record (with its fixes and pending state)",
	Long: "delete removes one error record for good, including its recorded\n" +
		"solutions and pending state. It asks for confirmation; --yes skips\n" +
		"the prompt (for scripts).",
	Example: "  err delete 3\n  err delete 3 --yes",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer termx.PlainUnlessTTY(cmd.ErrOrStderr())()
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id %q", args[0])
		}

		dbPath, err := config.DBPath()
		if err != nil {
			return err
		}
		st, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer st.Close()

		e, err := st.Get(id)
		if err != nil {
			return err
		}
		if e == nil {
			return fmt.Errorf("error #%d not found", id)
		}

		// Always show what is about to be destroyed.
		printTargetSummary(cmd.ErrOrStderr(), "delete", e)

		if !deleteYes {
			ok, err := confirmDestructive(cmd.InOrStdin(), cmd.ErrOrStderr(),
				fmt.Sprintf("delete error #%d? [y/N] ", id), false)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(cmd.ErrOrStderr(), termx.Faint("--err-- aborted, nothing deleted"))
				return nil
			}
		}

		if _, err := st.DeleteError(id); err != nil {
			return err
		}
		fmt.Fprintln(cmd.ErrOrStderr(), termx.Faint("--err-- deleted error #"+strconv.FormatInt(id, 10)))
		return nil
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteYes, "yes", false, "skip the confirmation prompt")
	rootCmd.AddCommand(deleteCmd)
}
