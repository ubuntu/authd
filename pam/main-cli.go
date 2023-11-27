//go:build pam_binary_cli

package main

import (
	"fmt"
	"os"

	"github.com/msteinert/pam"
	"github.com/sirupsen/logrus"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/pam/pam_test"
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

	authResult := module.Authenticate(mTx, pam.Flags(0), nil)
	fmt.Println("Auth return:", authResult)

	// Simulate setting auth broker as default.
	accMgmtResult := module.AcctMgmt(mTx, pam.Flags(0), nil)
	fmt.Println("Acct mgmt return:", accMgmtResult)
}
