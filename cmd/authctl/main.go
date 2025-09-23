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

var rootCmd = &cobra.Command{
	Use:   "authctl",
	Short: "CLI tool to interact with authd",
	Long:  "authctl is a command-line tool to interact with the authd service for user and group management.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// The command was successfully parsed, so we don't want cobra to print usage information on error.
		cmd.SilenceUsage = true
	},
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	// We handle errors ourselves
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE:          func(cmd *cobra.Command, args []string) error { return cmd.Usage() },
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

		// If the error is a gRPC status, we print the message and exit with the gRPC status code.
		switch s.Code() {
		case codes.PermissionDenied:
			fmt.Fprintln(os.Stderr, "Permission denied:", s.Message())
		default:
			fmt.Fprintln(os.Stderr, "Error:", s.Message())
		}
		code := int(s.Code())
		if code < 0 || code > 255 {
			// We cannot exit with a negative code or a code greater than 255,
			// so we map it to 1 in that case.
			code = 1
		}

		os.Exit(code)
	}
}
