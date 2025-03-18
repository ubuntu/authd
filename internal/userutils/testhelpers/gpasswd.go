// This is a small wrapper around the gpasswd command, which allows to run
// gpasswd on a group file that's not at /etc/group. It does this by:
//   - Reading the path to the group file from the GROUP_FILE environment variable.
//   - Running the gpasswd command in a bubblewrap sandbox with the group file
//     bind-mounted to /etc/group.
//
// The group file must be named "group".
package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	log.SetFlags(log.Lshortfile)

	groupFile := os.Getenv("GROUP_FILE")
	if groupFile == "" {
		log.Fatalf("Error: GROUP_FILE environment variable is not set.")
	}
	log.Printf("GROUP_FILE: %s", groupFile)

	if filepath.Base(groupFile) != "group" {
		log.Fatalf("Error: The group file must be named 'group'.")
	}

	bwrapPath, err := exec.LookPath("bwrap")
	if err != nil {
		log.Fatalf("Error: bwrap not found in PATH: %v", err)
	}

	args := []string{
		bwrapPath,
		"--unshare-user",
		"--uid", "0",
		"--ro-bind", "/", "/",
		"--bind", filepath.Dir(groupFile), "/etc",
		"--ro-bind", "/etc/passwd", "/etc/passwd",
		"gpasswd",
	}

	// Add the gpasswd command arguments
	args = append(args, os.Args[1:]...)

	log.Printf("Executing command: %s", strings.Join(args, " "))
	//nolint:gosec // G204 there is no security issue with the arguments passed to syscall.Exec
	err = syscall.Exec(args[0], args, os.Environ())
	if err != nil {
		log.Fatalf("Error executing command: %v", err)
	}
}
