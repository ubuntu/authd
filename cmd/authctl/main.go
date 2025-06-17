// Package main implements Cobra commands for management operations on authd.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/ubuntu/authd/cmd/authctl/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const cmdName = "authctl"

var rootCmd = &cobra.Command{
	Use:   fmt.Sprintf("%s COMMAND", cmdName),
	Short: "CLI tool to interact with authd",
	Long:  "authctl is a CLI tool which can be used to interact with authd.",
	Args:  cobra.NoArgs,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
}

func init() {
	// Disable command sorting by name. This makes cobra print the commands in the
	// order they are added to the root command and adds the `help` and `completion`
	// commands at the end.
	cobra.EnableCommandSorting = false

	rootCmd.AddCommand(user.UserCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		s, ok := status.FromError(err)
		if !ok {
			// If the error is not a gRPC status, we print it as is.
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		// If the error is a gRPC status, we print the message and exit with the appropriate code.
		switch s.Code() {
		case codes.PermissionDenied:
			fmt.Fprintln(os.Stderr, "Permission denied:", s.Message())
		default:
			fmt.Fprintln(os.Stderr, "Error:", s.Message())
		}

		os.Exit(int(s.Code()))
	}
}
