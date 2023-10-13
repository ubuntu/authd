//go:generate go run github.com/msteinert/pam/cmd/pam-moduler -libname "pam_authd.so" -no-main -type pamModule
//go:generate go generate --skip="pam_module.go"
//go:generate sh -c "cc -o go-loader/pam_go_loader.so go-loader/module.c -Wl,--as-needed -Wl,--allow-shlib-undefined -shared -fPIC -Wl,--unresolved-symbols=report-all -lpam && chmod 600 go-loader/pam_go_loader.so"

// Package main is the package for the PAM library.
package main

import "C"

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/msteinert/pam"
	"github.com/sirupsen/logrus"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/log"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// pamModule is the structure that implements the pam.ModuleHandler interface
// that is called during pam operations.
type pamModule struct {
}

var (
	// brokerIDUsedToAuthenticate global variable is for the second stage authentication to select the default broker for the current user.
	brokerIDUsedToAuthenticate string
)

/*
	FIXME: provide instructions using pam-auth-update instead!
	Add to /etc/pam.d/common-auth
	auth    [success=3 default=die ignore=ignore]   pam_authd.so
*/

// Authenticate is the method that is invoked during pam_authenticate request.
func (h *pamModule) Authenticate(mTx pam.ModuleTransaction, flags pam.Flags, args []string) error {
	// Initialize localization
	// TODO

	// Attach logger and info handler.
	// TODO

	interactiveTerminal := term.IsTerminal(int(os.Stdin.Fd()))

	client, closeConn, err := newClient(args)
	if err != nil {
		log.Debug(context.TODO(), err)
		return pam.ErrAuthinfoUnavail
	}
	defer closeConn()

	appState := model{
		pamMTx:              mTx,
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
		return pam.ErrAbort
	}

	logErrMsg := "unknown"
	errCode := pam.ErrSystem

	switch exitMsg := appState.exitMsg.(type) {
	case pamSuccess:
		brokerIDUsedToAuthenticate = exitMsg.brokerID
		return nil
	case pamIgnore:
		// localBrokerID is only set on pamIgnore if the user has chosen local broker.
		brokerIDUsedToAuthenticate = exitMsg.localBrokerID
		if exitMsg.String() != "" {
			log.Debugf(context.TODO(), "Ignoring authd authentication: %s", exitMsg)
		}
		logErrMsg = ""
		errCode = pam.ErrIgnore
	case pamAbort:
		if exitMsg.String() != "" {
			logErrMsg = fmt.Sprintf("cancelled authentication: %s", exitMsg)
		}
		errCode = pam.ErrAbort
	case pamAuthError:
		if exitMsg.String() != "" {
			logErrMsg = fmt.Sprintf("authentication: %s", exitMsg)
		}
		errCode = pam.ErrAuth
	case pamAuthInfoUnavailable:
		if exitMsg.String() != "" {
			logErrMsg = fmt.Sprintf("missing authentication data: %s", exitMsg)
		}
		errCode = pam.ErrAuthinfoUnavail
	case pamSystemError:
		if exitMsg.String() != "" {
			logErrMsg = fmt.Sprintf("system: %s", exitMsg)
		}
		errCode = pam.ErrSystem
	}

	if logErrMsg != "" {
		fmt.Fprintf(os.Stderr, "Error: %v\n", logErrMsg)
	}

	return errCode
}

// AcctMgmt sets any used brokerID as default for the user.
func (h *pamModule) AcctMgmt(mTx pam.ModuleTransaction, flags pam.Flags, args []string) error {
	// Only set the brokerID as default if we stored one after authentication.
	if brokerIDUsedToAuthenticate == "" {
		return pam.ErrIgnore
	}

	// Get current user for broker
	user, err := mTx.GetItem(pam.User)
	if err != nil {
		return err
	}

	if user == "" {
		log.Infof(context.TODO(), "can't get user from PAM")
		return pam.ErrIgnore
	}

	client, closeConn, err := newClient(args)
	if err != nil {
		log.Debugf(context.TODO(), "%s", err)
		return pam.ErrIgnore
	}
	defer closeConn()

	req := authd.SDBFURequest{
		BrokerId: brokerIDUsedToAuthenticate,
		Username: user,
	}
	if _, err := client.SetDefaultBrokerForUser(context.TODO(), &req); err != nil {
		log.Infof(context.TODO(), "Can't set default broker  (%q) for %q: %v", brokerIDUsedToAuthenticate, user, err)
		return pam.ErrIgnore
	}

	return nil
}

// newClient returns a new GRPC client ready to emit requests.
func newClient(args []string) (client authd.PAMClient, close func(), err error) {
	conn, err := grpc.Dial("unix://"+getSocketPath(args), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to authd: %v", err)
	}
	return authd.NewPAMClient(conn), func() { conn.Close() }, nil
}

// getSocketPath returns the socket path to connect to which can be overridden manually.
func getSocketPath(args []string) string {
	socketPath := consts.DefaultSocketPath
	for _, arg := range args {
		opt, optarg, _ := strings.Cut(arg, "=")
		switch opt {
		case "socket":
			socketPath = optarg
		default:
		}
	}
	return socketPath
}

// SetCred is the method that is invoked during pam_setcred request.
func (h *pamModule) SetCred(pam.ModuleTransaction, pam.Flags, []string) error {
	return pam.ErrIgnore
}

// ChangeAuthTok is the method that is invoked during pam_chauthtok request.
func (h *pamModule) ChangeAuthTok(pam.ModuleTransaction, pam.Flags, []string) error {
	return pam.ErrIgnore
}

// OpenSession is the method that is invoked during pam_open_session request.
func (h *pamModule) OpenSession(pam.ModuleTransaction, pam.Flags, []string) error {
	return pam.ErrIgnore
}

// CloseSession is the method that is invoked during pam_close_session request.
func (h *pamModule) CloseSession(pam.ModuleTransaction, pam.Flags, []string) error {
	return pam.ErrIgnore
}

// go_pam_cleanup_module is called by the go-loader PAM module during onload.
//
//export go_pam_cleanup_module
func go_pam_cleanup_module() {
	runtime.GC()
}

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

	authResult := module.Authenticate(nil, pam.Flags(0), nil)
	fmt.Println("Auth return:", authResult)

	// Simulate setting auth broker as default.
	accMgmtResult := module.AcctMgmt(nil, pam.Flags(0), nil)
	fmt.Println("Acct mgmt return:", accMgmtResult)
}
