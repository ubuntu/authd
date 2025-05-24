package user

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ubuntu/authd/internal/proto/authd"
)

// lockCmd is a command to disable a user.
var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock (disable) a user managed by authd",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Disabling user %q\n...", args[0])

		client, err := NewUserServiceClient()
		if err != nil {
			return err
		}

		_, err = client.DisableUser(context.Background(), &authd.DisableUserRequest{Name: args[0]})
		if err != nil {
			return err
		}

		return nil
	},
}
