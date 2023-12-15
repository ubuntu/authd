// Package main is the package for the PAM library.
package main

import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/pam/internal/adapter"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// pamModule is the structure that implements the pam.ModuleHandler interface
// that is called during pam operations.
type pamModule struct {
}

const (
	// authenticationBrokerIDKey is the Key used to store the data in the
	// PAM module for the second stage authentication to select the default
	// broker for the current user.
	authenticationBrokerIDKey = "authentication-broker-id"
)

func showPamMessage(mTx pam.ModuleTransaction, style pam.Style, msg string) error {
	switch style {
	case pam.TextInfo, pam.ErrorMsg:
	default:
		return fmt.Errorf("message style not supported: %v", style)
	}
	if _, err := mTx.StartStringConv(style, msg); err != nil {
		log.Errorf(context.TODO(), "Failed sending message to pam: %v", err)
		return err
	}
	return nil
}

func sendReturnMessageToPam(mTx pam.ModuleTransaction, retStatus adapter.PamReturnStatus) {
	msg := retStatus.Message()
	if msg == "" {
		return
	}

	style := pam.ErrorMsg
	switch retStatus.(type) {
	case adapter.PamIgnore, adapter.PamSuccess:
		style = pam.TextInfo
	}

	_ = showPamMessage(mTx, style, msg)
}

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

	appState := adapter.UIModel{
		PamMTx:              mTx,
		Client:              client,
		InteractiveTerminal: interactiveTerminal,
	}

	if err := mTx.SetData(authenticationBrokerIDKey, nil); err != nil {
		return err
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

	sendReturnMessageToPam(mTx, appState.ExitStatus())

	switch exitStatus := appState.ExitStatus().(type) {
	case adapter.PamSuccess:
		if err := mTx.SetData(authenticationBrokerIDKey, exitStatus.BrokerID); err != nil {
			return err
		}
		return nil

	case adapter.PamIgnore:
		// localBrokerID is only set on pamIgnore if the user has chosen local broker.
		if err := mTx.SetData(authenticationBrokerIDKey, exitStatus.LocalBrokerID); err != nil {
			return err
		}
		return fmt.Errorf("%w: %s", exitStatus.Status(), exitStatus.Message())

	case adapter.PamReturnError:
		return fmt.Errorf("%w: %s", exitStatus.Status(), exitStatus.Message())
	}

	return fmt.Errorf("%w: unknown exit code", pam.ErrSystem)
}

// AcctMgmt sets any used brokerID as default for the user.
func (h *pamModule) AcctMgmt(mTx pam.ModuleTransaction, flags pam.Flags, args []string) error {
	brokerData, err := mTx.GetData(authenticationBrokerIDKey)
	if err != nil && errors.Is(err, pam.ErrNoModuleData) {
		return pam.ErrIgnore
	}

	brokerIDUsedToAuthenticate, ok := brokerData.(string)
	if !ok {
		msg := fmt.Sprintf("broker data as an invalid type %#v", brokerData)
		log.Errorf(context.TODO(), msg)
		_ = showPamMessage(mTx, pam.ErrorMsg, msg)
		return pam.ErrIgnore
	}

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
