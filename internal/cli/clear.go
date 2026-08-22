package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/errata/internal/config"
	"github.com/wumuwutu/errata/internal/store"
	"github.com/wumuwutu/errata/internal/termx"
)

var clearYes bool

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete ALL error data, back to a pristine library",
	Long: "clear wipes every recorded error (with fixes, pending state and the\n" +
		"search index) and resets the id sequence. Your config and ignore list\n" +
		"are kept. It asks for the full word \"yes\"; --yes skips the prompt.",
	Example: "  err clear\n  err clear --yes",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer termx.PlainUnlessTTY(cmd.ErrOrStderr())()
		dbPath, err := config.DBPath()
		if err != nil {
			return err
		}
		st, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer st.Close()

		_, total, err := st.RecordRate()
		if err != nil {
			return err
		}
		if total == 0 {
			fmt.Fprintln(cmd.ErrOrStderr(), termx.Faint("--err-- nothing to clear"))
			return nil
		}

		if !clearYes {
			ok, err := confirmDestructive(cmd.InOrStdin(), cmd.ErrOrStderr(),
				"clear ALL "+strconv.Itoa(total)+" error records? type yes to confirm: ", true)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(cmd.ErrOrStderr(), termx.Faint("--err-- aborted, nothing cleared"))
				return nil
			}
		}

		n, err := st.ClearAll()
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.ErrOrStderr(), termx.Faint("--err-- cleared "+strconv.FormatInt(n, 10)+
			" error records - library back to pristine state"))
		return nil
	},
}

func init() {
	clearCmd.Flags().BoolVar(&clearYes, "yes", false, "skip the confirmation prompt")
	rootCmd.AddCommand(clearCmd)
}
