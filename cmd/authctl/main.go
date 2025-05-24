// Package main implements Cobra commands for management operations on authd.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/ubuntu/authd/cmd/authctl/user"
)

const cmdName = "authctl"

var rootCmd = &cobra.Command{
	Use:   fmt.Sprintf("%s COMMAND", cmdName),
	Short: "CLI tool to interact with authd",
	Long:  "authctl is a CLI tool which can be used to interact with authd.",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) {},
}

func init() {
	rootCmd.AddCommand(user.UserCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
