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
	Short: "CLI tool to interact with authd services",
	Long:  "authctl is a CLI tool which can be used to interact with various authd services.",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) {},
}

func init() {
	rootCmd.AddCommand(user.UserCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error while executing authctl '%s'\n", err)
		os.Exit(1)
	}
}
