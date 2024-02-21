// Package main is the package for the PAM library.
package main

import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/pam/internal/adapter"
	"github.com/ubuntu/authd/pam/internal/gdm"
	"golang.org/x/term"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
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

var supportedArgs = []string{
	"socket", // The authd socket to connect to.
}

func parseArgs(args []string) map[string]string {
	parsed := make(map[string]string)

	for _, arg := range args {
		opt, value, _ := strings.Cut(arg, "=")
		parsed[opt] = value

		if !slices.Contains(supportedArgs, opt) {
			log.Warningf(context.TODO(), "Provided argument %q is not supported and will be ignored", arg)
		}
	}

	return parsed
}

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

	if err := showPamMessage(mTx, style, msg); err != nil {
		log.Warningf(context.TODO(), "Impossible to send PAM message: %v", err)
	}
}

// Authenticate is the method that is invoked during pam_authenticate request.
func (h *pamModule) Authenticate(mTx pam.ModuleTransaction, flags pam.Flags, args []string) error {
	return h.handleAuthRequest(authd.SessionMode_AUTH, mTx, flags, args)
}

// ChangeAuthTok is the method that is invoked during pam_sm_chauthtok request.
func (h *pamModule) ChangeAuthTok(mTx pam.ModuleTransaction, flags pam.Flags, args []string) error {
	return h.handleAuthRequest(authd.SessionMode_PASSWD, mTx, flags, args)
}

func (h *pamModule) handleAuthRequest(mode authd.SessionMode, mTx pam.ModuleTransaction, flags pam.Flags, args []string) error {
	// Initialize localization
	// TODO

	// Attach logger and info handler.
	// TODO

	var pamClientType adapter.PamClientType
	var teaOpts []tea.ProgramOption

	if gdm.IsPamExtensionSupported(gdm.PamExtensionCustomJSON) {
		// Explicitly set the output to something so that the program
		// won't try to init some terminal fancy things that also appear
		// to be racy...
		// See: https://github.com/charmbracelet/bubbletea/issues/910
		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return errors.Join(err, pam.ErrSystem)
		}
		pamClientType = adapter.Gdm
		teaOpts = append(teaOpts,
			tea.WithInput(nil),
			tea.WithoutRenderer(),
			tea.WithoutSignals(),
			tea.WithoutSignalHandler(),
			tea.WithoutCatchPanics(),
			tea.WithOutput(devNull),
		)
	} else if term.IsTerminal(int(os.Stdin.Fd())) {
		pamClientType = adapter.InteractiveTerminal
	} else {
		return fmt.Errorf("pam module used through an unsupported client: %w", pam.ErrSystem)
	}

	client, closeConn, err := newClient(parseArgs(args))
	if err != nil {
		log.Debug(context.TODO(), err)
		if err := showPamMessage(mTx, pam.ErrorMsg, err.Error()); err != nil {
			log.Warningf(context.TODO(), "Impossible to show PAM message: %v", err)
		}
		return errors.Join(err, pam.ErrAuthinfoUnavail)
	}
	defer closeConn()

	appState := adapter.UIModel{
		PamMTx:      mTx,
		Client:      client,
		ClientType:  pamClientType,
		SessionMode: mode,
	}

	if err := mTx.SetData(authenticationBrokerIDKey, nil); err != nil {
		return err
	}

	teaOpts = append(teaOpts, tea.WithFilter(appState.MsgFilter))
	p := tea.NewProgram(&appState, teaOpts...)
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
		if err := showPamMessage(mTx, pam.ErrorMsg, msg); err != nil {
			log.Warningf(context.TODO(), "Impossible to show PAM message: %v", err)
		}

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

	client, closeConn, err := newClient(parseArgs(args))
	if err != nil {
		log.Debugf(context.TODO(), "%s", err)
		return pam.ErrAuthinfoUnavail
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
func newClient(args map[string]string) (client authd.PAMClient, close func(), err error) {
	conn, err := grpc.Dial("unix://"+getSocketPath(args), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to authd: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.TODO(), time.Second*5)
	defer cancel()
	for conn.GetState() != connectivity.Ready {
		if !conn.WaitForStateChange(waitCtx, conn.GetState()) {
			conn.Close()
			return nil, func() {}, fmt.Errorf("could not connect to authd: %w", waitCtx.Err())
		}
	}
	return authd.NewPAMClient(conn), func() { conn.Close() }, nil
}

// getSocketPath returns the socket path to connect to which can be overridden manually.
func getSocketPath(args map[string]string) string {
	if val, ok := args["socket"]; ok {
		return val
	}
	return consts.DefaultSocketPath
}

// SetCred is the method that is invoked during pam_setcred request.
func (h *pamModule) SetCred(pam.ModuleTransaction, pam.Flags, []string) error {
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
