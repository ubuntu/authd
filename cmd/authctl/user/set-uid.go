package user

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/ubuntu/authd/internal/proto/authd"
)

// setUIDCmd is a command to set the UID of a user managed by authd.
var setUIDCmd = &cobra.Command{
	Use:   "set-uid <name> <uid>",
	Short: "Set the UID of a user managed by authd",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		uidStr := args[1]
		uid, err := strconv.ParseUint(uidStr, 10, 32)
		if err != nil {
			return err
		}

		fmt.Printf("Setting UID of user %q to %d...\n", name, uid)

		client, err := NewUserServiceClient()
		if err != nil {
			return err
		}

		_, err = client.SetUserID(context.Background(), &authd.SetUserIDRequest{
			Name: name,
			Id:   uint32(uid),
		})
		if err != nil {
			return err
		}

		return nil
	},
}
