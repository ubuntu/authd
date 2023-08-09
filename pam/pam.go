package main

/*
#cgo LDFLAGS: -lpam -fPIC
#include <security/pam_appl.h>
#include <security/pam_ext.h>
#include <stdlib.h>
#include <string.h>

char *string_from_argv(int i, char **argv);
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sirupsen/logrus"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/log"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

//go:generate sh -c "go build -ldflags='-extldflags -Wl,-soname,pam_authd.so' -buildmode=c-shared -o pam_authd.so"

/*
	Add to /etc/pam.d/common-auth
	auth    [success=3 default=die ignore=ignore]   pam_authd.so
*/

//export pam_sm_authenticate
func pam_sm_authenticate(pamh *C.pam_handle_t, flags, argc C.int, argv **C.char) C.int {
	// Initialize localization
	// TODO

	// Attach logger and info handler.
	// TODO
	log.SetLevel(log.DebugLevel)
	f, err := os.OpenFile("/tmp/logdebug", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	logrus.SetOutput(f)

	// Check if we are in an interactive terminal to see if we can do something
	/*if !term.IsTerminal(int(os.Stdin.Fd())) {
		log.Info(context.TODO(), "Not in an interactive terminal and not an authd compatible application. Exiting")
		return C.PAM_IGNORE
	}*/

	interactiveTerminal := term.IsTerminal(int(os.Stdin.Fd()))

	client, closeConn, err := newClient(argc, argv)
	if err != nil {
		log.Debug(context.TODO(), err)
		return C.PAM_IGNORE
	}
	defer closeConn()

	appState := model{
		pamh:                pamh,
		client:              client,
		interactiveTerminal: interactiveTerminal,
	}

	//tea.WithInput(nil)
	//tea.WithoutRenderer()
	var opts []tea.ProgramOption
	if !interactiveTerminal {
		opts = append(opts, tea.WithInput(nil), tea.WithoutRenderer())
	}
	p := tea.NewProgram(&appState, opts...)
	if _, err := p.Run(); err != nil {
		log.Errorf(context.TODO(), "Cancelled authentication: %v", err)
		return C.PAM_ABORT
	}

	logErrMsg := "unknown"
	var errCode C.int = C.PAM_SYSTEM_ERR

	switch exitMsg := appState.exitMsg; exitMsg.(type) {
	case pamSuccess:
		return C.PAM_SUCCESS
	case pamIgnore:
		if exitMsg.ExitMsg() != "" {
			log.Debugf(context.TODO(), "Ignoring authd authentication: %v", exitMsg)
		}
		logErrMsg = ""
		errCode = C.PAM_IGNORE
	case pamAbort:
		if exitMsg.ExitMsg() != "" {
			logErrMsg = fmt.Sprintf("cancelled authentication: %v", exitMsg)
		}
		errCode = C.PAM_ABORT
	case pamAuthError:
		if exitMsg.ExitMsg() != "" {
			logErrMsg = fmt.Sprintf("authentication: %v", exitMsg)
		}
		errCode = C.PAM_AUTH_ERR
	case pamSystemError:
		if exitMsg.ExitMsg() != "" {
			logErrMsg = fmt.Sprintf("system: %v", exitMsg)
		}
		errCode = C.PAM_SYSTEM_ERR
	}

	if logErrMsg != "" {
		fmt.Fprintf(os.Stderr, "Error: %v\n", logErrMsg)
	}

	return errCode
}

//export pam_sm_acct_mgmt
func pam_sm_acct_mgmt(pamh *C.pam_handle_t, flags, argc C.int, argv **C.char) C.int {
	/*client, closeConn, err := newClient(argc, argv)
	if err != nil {
		log.Debugf(context.TODO(), "%s", err)
		return C.PAM_IGNORE
	}
	defer closeConn()

	// Get current user for broker.
	user, err := getUser(pamh, "")
	if err != nil {
		log.Infof(context.TODO(), "Can't get user: %v", err)
		return C.PAM_IGNORE
	}

	req := authd.SDBFURequest{
		SessionId: sessionID,
		Username:  user,
	}
	if _, err := client.SetDefaultBrokerForUser(context.TODO(), &req); err != nil {
		log.Infof(context.TODO(), "Can't set default broker for %q on session %q: %v", user, sessionID, err)
		return C.PAM_IGNORE
	}*/

	return C.PAM_SUCCESS
}

// newClient returns a new GRPC client ready to emit requests
func newClient(argc C.int, argv **C.char) (client authd.PAMClient, close func(), err error) {
	conn, err := grpc.Dial("unix://"+getSocketPath(argc, argv), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to authd: %v", err)
	}
	return authd.NewPAMClient(conn), func() { conn.Close() }, nil
}

// getSocketPath returns the socket path to connect to which can be overridden manually.
func getSocketPath(argc C.int, argv **C.char) string {
	socketPath := consts.DefaultSocketPath
	for _, arg := range sliceFromArgv(argc, argv) {
		opt, optarg, _ := strings.Cut(arg, "=")
		switch opt {
		case "socket":
			socketPath = optarg
		default:
		}
	}
	return socketPath
}

//export pam_sm_setcred
func pam_sm_setcred(pamh *C.pam_handle_t, flags, argc C.int, argv **C.char) C.int {
	return C.PAM_IGNORE
}

func main() {
	fmt.Println("RETURN: ", pam_sm_authenticate(nil, 0, 0, nil))
}
