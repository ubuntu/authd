// Package user provides utilities for managing user operations.
package user

import (
	"github.com/spf13/cobra"
)

// UserCmd is a command to perform user-related operations.
var UserCmd = &cobra.Command{
	Use:   "user",
	Short: "Commands retaled to users",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) {},
}

func init() {
	UserCmd.AddCommand(DisableCmd)
	UserCmd.AddCommand(EnableCmd)
}
