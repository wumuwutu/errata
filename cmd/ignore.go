package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wumuwutu/dejavu/internal/config"
)

var (
	ignoreCommand string
	ignoreDir     string
)

var ignoreCmd = &cobra.Command{
	Use:   "ignore",
	Short: "Manage the recording blacklist (commands and directories)",
	Long: "Errors raised by blacklisted commands, or under blacklisted directory\n" +
		"prefixes, are never recorded. Stored in ~/.config/dejavu/config.yaml.",
}

var ignoreAddCmd = &cobra.Command{
	Use:     "add (--command <name> | --dir <path-prefix>)",
	Short:   "Add a command name or directory prefix to the blacklist",
	Example: "  err ignore add --command npm\n  err ignore add --dir ~/work/secrets",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		kind, value, err := ignoreTarget()
		if err != nil {
			return err
		}
		if err := config.AddIgnore(kind, value); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "err: ignoring %s %q\n", kind, value)
		return nil
	},
}

var ignoreRemoveCmd = &cobra.Command{
	Use:     "remove (--command <name> | --dir <path-prefix>)",
	Short:   "Remove an entry from the blacklist",
	Example: "  err ignore remove --command npm",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		kind, value, err := ignoreTarget()
		if err != nil {
			return err
		}
		removed, err := config.RemoveIgnore(kind, value)
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("%s %q was not blacklisted", kind, value)
		}
		fmt.Fprintf(os.Stderr, "err: removed %s %q from blacklist\n", kind, value)
		return nil
	},
}

var ignoreListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the blacklist",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		fmt.Println("commands:")
		for _, c := range cfg.IgnoreCommands {
			fmt.Println("  " + c)
		}
		fmt.Println("directories:")
		for _, d := range cfg.IgnoreDirs {
			fmt.Println("  " + d)
		}
		return nil
	},
}

func init() {
	for _, sub := range []*cobra.Command{ignoreAddCmd, ignoreRemoveCmd} {
		sub.Flags().StringVar(&ignoreCommand, "command", "", "command basename to match (e.g. npm)")
		sub.Flags().StringVar(&ignoreDir, "dir", "", "directory prefix to match (e.g. ~/work/secrets)")
	}
	ignoreCmd.AddCommand(ignoreAddCmd, ignoreRemoveCmd, ignoreListCmd)
	rootCmd.AddCommand(ignoreCmd)
}

func ignoreTarget() (config.IgnoreKind, string, error) {
	switch {
	case ignoreCommand != "" && ignoreDir != "":
		return "", "", errors.New("pass either --command or --dir, not both")
	case ignoreCommand != "":
		return config.IgnoreCommand, ignoreCommand, nil
	case ignoreDir != "":
		return config.IgnoreDir, ignoreDir, nil
	default:
		return "", "", errors.New("pass --command <name> or --dir <path-prefix>")
	}
}
