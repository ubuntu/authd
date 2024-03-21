//go:build pam_binary_cli

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

// Simulating pam on the CLI for manual testing.
func main() {
	logDir := os.Getenv("AUTHD_PAM_CLI_LOG_DIR")

	module := &pamModule{}
	mTx := pam_test.NewModuleTransactionDummy(pam.ConversationFunc(
		func(style pam.Style, msg string) (string, error) {
			switch style {
			case pam.TextInfo:
				fmt.Fprintf(os.Stderr, "PAM Info Message: %s\n", msg)
			case pam.ErrorMsg:
				fmt.Fprintf(os.Stderr, "PAM Error Message: %s\n", msg)
			default:
				return "", fmt.Errorf("PAM style %d not implemented", style)
			}
			return "", nil
		}))

	var resultMsg string
	var pamFunc pam.ModuleHandlerFunc
	var pamFlags pam.Flags
	action, args := os.Args[1], os.Args[2:]
	switch action {
	case "login":
		pamFunc = module.Authenticate
		resultMsg = "PAM Authenticate() for user %q"
	case "passwd":
		pamFunc = module.ChangeAuthTok
		pamFlags = pam.UpdateAuthtok
		resultMsg = "PAM ChangeAuthTok() for user %q"
	default:
		panic("Unknown PAM operation: " + action)
	}

	defaultArgs := []string{"debug=true"}
	if logDir != "" {
		logPath := filepath.Join(logDir, "authd-pam-cli.log")
		defaultArgs = append(defaultArgs, "logfile="+logPath)
	}

	if action == "passwd" {
		// Do a preliminary check as PAM does first.
		err := pamFunc(mTx, pam.Silent|pam.PrelimCheck, args)
		if err != nil {
			log.Fatalf("PAM ChangeAuthTok(), unexpected error: %v", err)
		}
	}

	args = append(defaultArgs, args...)
	pamRes := pamFunc(mTx, pamFlags, args)
	user, _ := mTx.GetItem(pam.User)

	printPamResult(fmt.Sprintf(resultMsg, user), pamRes)

	// Simulate setting auth broker as default.
	printPamResult("PAM AcctMgmt()", module.AcctMgmt(mTx, pam.Flags(0), args))
}

func printPamResult(action string, result error) {
	var pamErr pam.Error
	if errors.As(result, &pamErr) {
		fmt.Printf("%s exited with error (PAM exit code: %d): %v\n", action, pamErr, result)
		return
	}
	if result != nil {
		fmt.Printf("%s exited with error: %v\n", action, result)
		return
	}
	fmt.Printf("%s exited with success\n", action)
}
