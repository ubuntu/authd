// This is a small wrapper around the gpasswd command, which allows to run
// gpasswd on a group file that's not at /etc/group. It does this by:
//   - Reading the path to the group file from the GROUP_FILE environment variable.
//   - Running the gpasswd command in a bubblewrap sandbox with the group file
//     bind-mounted to /etc/group.
//
// The group file must be named "group".

//go:build testutils_testhelpers

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	log.SetFlags(log.Lshortfile)

	testData := os.Getenv("BUBBLEWRAP_TEST_DATA")
	if testData == "" {
		log.Fatalf("Error: BUBBLEWRAP_TEST_DATA environment variable is not set.")
	}
	fmt.Fprintf(os.Stderr, "BUBBLEWRAP_TEST_DATA: %s\n", testData)

	bwrapPath, err := exec.LookPath("bwrap")
	if err != nil {
		log.Fatalf("Error: bwrap not found in PATH: %v", err)
	}

	etcDir := filepath.Join(testData, "etc")
	var perms os.FileMode = 0700
	if os.Getenv("SUDO_UID") != "" {
		// If running in sudo we may want to give the folder more permissions
		// not to lock out the caller.
		perms = 0777
	}
	err = os.MkdirAll(etcDir, perms)
	if err != nil {
		log.Fatalf("Error: Impossible to create /etc: %v", err)
	}

	bwrapArgs := []string{
		bwrapPath,
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--bind", os.TempDir(), os.TempDir(),
		"--bind", testData, testData,
		"--bind", etcDir, "/etc",

		// Bind relevant etc files. We go manual here, since there's no
		// need to get much more than those, while we could in theory just
		// bind everything that is in host, and excluding the ones we want
		// to override.
		"--ro-bind", "/etc/environment", "/etc/environment",
		"--ro-bind", "/etc/localtime", "/etc/localtime",
		"--ro-bind", "/etc/login.defs", "/etc/login.defs",
		"--ro-bind", "/etc/nsswitch.conf", "/etc/nsswitch.conf",
		"--ro-bind", "/etc/passwd", "/etc/passwd",
		"--ro-bind", "/etc/shadow", "/etc/shadow",
		"--ro-bind", "/etc/subgid", "/etc/subgid",
		"--ro-bind", "/etc/sudo.conf", "/etc/sudo.conf",
		"--ro-bind", "/etc/sudoers", "/etc/sudoers",
		"--ro-bind", "/etc/timezone", "/etc/timezone",
		"--ro-bind", "/etc/pam.d", "/etc/pam.d",
		"--ro-bind", "/etc/security", "/etc/security",
	}

	if e := os.Getenv("GOCOVERDIR"); e != "" {
		bwrapArgs = append(bwrapArgs, "--bind", e, e)
	}

	if os.Geteuid() != 0 {
		bwrapArgs = append(bwrapArgs, "--unshare-user", "--uid", "0")
	}

	// Add the gpasswd command arguments
	args := append(bwrapArgs, os.Args[1:]...)

	fmt.Fprintf(os.Stderr, "Executing command: %s\n", strings.Join(args, " "))

	//nolint:gosec // G204 there is no security issue with the arguments passed to syscall.Exec
	err = syscall.Exec(args[0], args, os.Environ())
	if err != nil {
		log.Fatalf("Error executing command: %v", err)
	}
}
