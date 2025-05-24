package user

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ubuntu/authd/internal/proto/authd"
)

// unlockCmd is a command to enable a user.
var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock (enable) a user managed by authd",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Enabling user %q...\n", args[0])

		client, err := NewUserServiceClient()
		if err != nil {
			return err
		}

		_, err = client.EnableUser(context.Background(), &authd.EnableUserRequest{Name: args[0]})
		if err != nil {
			return err
		}

		return nil
	},
}
