// Package user provides utilities for managing user operations.
package user

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/proto/authd"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// UserCmd is a command to perform user-related operations.
var UserCmd = &cobra.Command{
	Use:   "user",
	Short: "Commands related to users",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Usage() },
}

// NewUserServiceClient creates and returns a new [authd.UserServiceClient].
func NewUserServiceClient() (authd.UserServiceClient, error) {
	authdSocket := os.Getenv("AUTHD_SOCKET")
	if authdSocket == "" {
		authdSocket = "unix://" + consts.DefaultSocketPath
	}

	conn, err := grpc.NewClient(authdSocket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to authd: %w", err)
	}

	client := authd.NewUserServiceClient(conn)
	return client, nil
}

func init() {
	UserCmd.AddCommand(lockCmd)
	UserCmd.AddCommand(unlockCmd)
}
