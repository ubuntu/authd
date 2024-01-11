//go:build integrationtests

package users

import (
	"os"
	"strings"
)

var defaultOptions options

func init() {
	args := os.Getenv("TESTS_GPASSWD_ARGS")
	grpFilePath := os.Getenv("TESTS_GPASSWD_GRP_FILE_PATH")
	if args == "" || grpFilePath == "" {
		panic("TESTS_GPASSWD_ARGS and TESTS_GPASSWD_GRP_FILE_PATH must be set")
	}

	defaultOptions = options{
		groupPath:    grpFilePath,
		gpasswdCmd:   strings.Split(args, "-sep-"),
		getUsersFunc: getPasswdUsernames,
	}
}
