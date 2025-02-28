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

var DisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable a user to log in",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Disabling user %s\n", args[0])
		conn, err := grpc.NewClient("unix://"+consts.DefaultSocketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))

		if err != nil {
			return err
		}

		client := authd.NewNSSClient(conn)
		_, err = client.EnablePasswd(context.Background(), &authd.EnablePasswdRequest{Name: args[0]})

		if err != nil {
			return err
		}

		return nil
	},
}
