package user

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/proto/authd"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// EnableCmd is a command to enable a user.
var EnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable a user managed by authd",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Enabling user %s\n", args[0])
		conn, err := grpc.NewClient("unix://"+consts.DefaultSocketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return err
		}

		client := authd.NewNSSClient(conn)
		_, err = client.EnableUser(context.Background(), &authd.EnableUserRequest{Name: args[0]})
		if err != nil {
			return err
		}

		return nil
	},
}
