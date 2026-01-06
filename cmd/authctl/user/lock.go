package user

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/ubuntu/authd/cmd/authctl/internal/client"
	"github.com/ubuntu/authd/cmd/authctl/internal/completion"
	"github.com/ubuntu/authd/internal/proto/authd"
)

// lockCmd is a command to lock (disable) a user.
var lockCmd = &cobra.Command{
	Use:               "lock <user>",
	Short:             "Lock (disable) a user managed by authd",
	Long:              `Lock a user so that it cannot log in.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completion.Users,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := client.NewUserServiceClient()
		if err != nil {
			return err
		}

		_, err = client.LockUser(context.Background(), &authd.LockUserRequest{Name: args[0]})
		if err != nil {
			return err
		}

		return nil
	},
}
