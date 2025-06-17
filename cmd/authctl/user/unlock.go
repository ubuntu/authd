package user

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ubuntu/authd/internal/proto/authd"
)

// unlockCmd is a command to unlock (enable) a user.
var unlockCmd = &cobra.Command{
	Use:   "unlock <user>",
	Short: "Unlock (enable) a user managed by authd",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Unlocking user %q...\n", args[0])

		client, err := NewUserServiceClient()
		if err != nil {
			return err
		}

		_, err = client.UnlockUser(context.Background(), &authd.UnlockUserRequest{Name: args[0]})
		if err != nil {
			return err
		}

		return nil
	},
}
