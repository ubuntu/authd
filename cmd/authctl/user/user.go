package user

import (
	"github.com/spf13/cobra"
)

var UserCmd = &cobra.Command{
	Use:   "user",
	Short: "Commands retaled to working with users",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) {},
}

func init() {
	UserCmd.AddCommand(DisableCmd)
	UserCmd.AddCommand(EnableCmd)
}
