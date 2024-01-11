//go:build pam_binary_cli

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/msteinert/pam/v2"
	"github.com/sirupsen/logrus"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/pam/internal/pam_test"
)

// Simulating pam on the CLI for manual testing.
func main() {
	log.SetLevel(log.DebugLevel)
	f, err := os.OpenFile("/tmp/logdebug", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	logrus.SetOutput(f)

	module := &pamModule{}
	mTx := pam_test.NewModuleTransactionDummy(pam.ConversationFunc(
		func(style pam.Style, msg string) (string, error) {
			switch style {
			case pam.TextInfo:
				fmt.Fprintf(os.Stderr, "PAM INFO: %s\n", msg)
			case pam.ErrorMsg:
				fmt.Fprintf(os.Stderr, "PAM ERROR: %s\n", msg)
			default:
				return "", fmt.Errorf("pam style %d not implemented", style)
			}
			return "", nil
		}))

	authResult := module.Authenticate(mTx, pam.Flags(0), os.Args)
	user, _ := mTx.GetItem(pam.User)
	printPamResult(fmt.Sprintf("PAM Authenticate() for user '%s'", user), authResult)

	// Simulate setting auth broker as default.
	printPamResult("PAM AcctMgmt()", module.AcctMgmt(mTx, pam.Flags(0), os.Args))
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
