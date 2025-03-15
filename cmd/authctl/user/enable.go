package user

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/proto/authd"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// enableCmd is a command to enable a user.
var enableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable a user managed by authd",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Enabling user %q\n", args[0])

		authdSocket := os.Getenv("AUTHD_SOCKET")
		if authdSocket == "" {
			authdSocket = "unix://" + consts.DefaultSocketPath
		}

		conn, err := grpc.NewClient(authdSocket, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
