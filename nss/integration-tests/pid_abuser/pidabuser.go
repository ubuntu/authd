// TiCS: disabled // This file is a test helper.

// Package main is the package for the pid abuser test tool.
package main

import (
	"fmt"
	"os"
	"os/user"
	"strconv"

	"gopkg.in/yaml.v3"
)

func main() {
	os.Setenv("AUTHD_PID", strconv.FormatInt(int64(os.Getpid()), 10))

	action := os.Getenv("ACTION")
	actionArg := os.Getenv("ACTION_ARG")

	switch action {
	case "lookup_user":
		outputAsYAMLOrFail(user.Lookup(actionArg))

	case "lookup_group":
		outputAsYAMLOrFail(user.LookupGroup(actionArg))

	case "lookup_uid":
		outputAsYAMLOrFail(user.LookupId(actionArg))

	case "lookup_gid":
		outputAsYAMLOrFail(user.LookupGroupId(actionArg))

	default:
		panic("Invalid action " + action)
	}
}

func outputAsYAMLOrFail[T any](val T, err error) {
	if err != nil {
		panic(err)
	}

	out, err := yaml.Marshal(val)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", out)
}
