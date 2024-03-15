// Package main is the package for the PAM library.
package main

import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/coreos/go-systemd/journal"
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
	authenticationBrokerIDKey = "authd.authentication-broker-id"

	// alreadyAuthenticatedKey is the Key used to store in the library that
	// we've already authenticated with this module and so that we should not
	// do this again.
	alreadyAuthenticatedKey = "authd.already-authenticated-flag"
)

var supportedArgs = []string{
	"debug",           // When this is set to "true", then debug logging is enabled.
	"logfile",         // The path of the file that will be used for logging.
	"disable_journal", // Disable logging on systemd journal (this is implicit when `logfile` is set).
	"socket",          // The authd socket to connect to.
	"force_reauth",    // Whether the authentication should be performed again even if it has been already completed.
}

// parseArgs parses the PAM arguments and returns a map of them and a function that logs the parsing issues.
// Such function should be called once the logger is setup, as the arguments may change the logging behavior.
func parseArgs(args []string) (map[string]string, func()) {
	parsed := make(map[string]string)
	var warnings []string

	for _, arg := range args {
		opt, value, _ := strings.Cut(arg, "=")
		parsed[opt] = value

		if !slices.Contains(supportedArgs, opt) {
			warnings = append(warnings,
				fmt.Sprintf("Provided argument %q is not supported and will be ignored", arg))
		}
	}

	return parsed, func() {
		for _, warn := range warnings {
			log.Warning(context.TODO(), warn)
		}
	}
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

// initLogging initializes the logging given the passed parameters.
// It returns a function that should be called in order to reset the logging to
// the default and potentially close the opened resources.
func initLogging(args map[string]string) (func(), error) {
	log.SetLevel(log.InfoLevel)
	resetLevel := func() {}
	if args["debug"] == "true" {
		log.SetLevel(log.DebugLevel)
		resetLevel = func() { log.SetLevel(log.InfoLevel) }
	}

	if out, ok := args["logfile"]; ok && out != "" {
		f, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0600)
		if err != nil {
			return resetLevel, err
		}
		log.SetOutput(f)
		return func() {
			resetLevel()
			log.SetOutput(os.Stderr)
			f.Close()
		}, nil
	}

	disableTerminalLogging := func() {
		if log.IsLevelEnabled(log.DebugLevel) {
			return
		}
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return
		}
		log.SetLevel(log.WarnLevel)
	}

	if !journal.Enabled() || args["disable_journal"] == "true" {
		log.SetHandler(nil)
		disableTerminalLogging()
		return resetLevel, nil
	}

	log.SetHandler(func(_ context.Context, level log.Level, format string, args ...interface{}) {
		journalPriority := journal.PriInfo
		switch level {
		case log.DebugLevel:
			journalPriority = journal.PriDebug
		case log.InfoLevel:
			journalPriority = journal.PriInfo
		case log.WarnLevel:
			journalPriority = journal.PriWarning
		case log.ErrorLevel:
			journalPriority = journal.PriErr
		}
		journal.Print(journalPriority, format, args...)
	})

	return func() {
		resetLevel()
		log.SetHandler(nil)
	}, nil
}

// Authenticate is the method that is invoked during pam_authenticate request.
func (h *pamModule) Authenticate(mTx pam.ModuleTransaction, flags pam.Flags, args []string) error {
	// Do not try to start authentication again if we've been already through this.
	// Since PAM modules can be stacked, so we may suffer reentry that is fine but it should
	// be explicitly allowed.
	parsedArgs, logArgsIssues := parseArgs(args)
	_, err := mTx.GetData(alreadyAuthenticatedKey)
	if err == nil && parsedArgs["force_reauth"] != "true" {
		return pam.ErrIgnore
	}
	if err != nil && !errors.Is(err, pam.ErrNoModuleData) {
		return err
	}

	err = h.handleAuthRequest(authd.SessionMode_AUTH, mTx, flags, parsedArgs, logArgsIssues)
	if err != nil && !errors.Is(err, pam.ErrIgnore) {
		return err
	}
	if err := mTx.SetData(alreadyAuthenticatedKey, struct{}{}); err != nil {
		return err
	}
	return err
}

// ChangeAuthTok is the method that is invoked during pam_sm_chauthtok request.
func (h *pamModule) ChangeAuthTok(mTx pam.ModuleTransaction, flags pam.Flags, args []string) error {
	parsedArgs, logArgsIssues := parseArgs(args)
	return h.handleAuthRequest(authd.SessionMode_PASSWD, mTx, flags, parsedArgs, logArgsIssues)
}

func (h *pamModule) handleAuthRequest(mode authd.SessionMode, mTx pam.ModuleTransaction, flags pam.Flags, parsedArgs map[string]string, logArgsIssues func()) error {
	// Initialize localization
	// TODO

	var pamClientType adapter.PamClientType
	var teaOpts []tea.ProgramOption

	closeLogging, err := initLogging(parsedArgs)
	defer closeLogging()
	if err != nil {
		return err
	}
	logArgsIssues()

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

	client, closeConn, err := newClient(parsedArgs)
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
	parsedArgs, logArgsIssues := parseArgs(args)
	closeLogging, err := initLogging(parsedArgs)
	defer closeLogging()
	if err != nil {
		return err
	}
	logArgsIssues()

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
		if err := showPamMessage(mTx, pam.ErrorMsg, "Can't get user from PAM"); err != nil {
			log.Warningf(context.TODO(), "Impossible to show PAM message: %v", err)
		}
		return pam.ErrIgnore
	}

	client, closeConn, err := newClient(parsedArgs)
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
		msg := fmt.Sprintf("Can't set default broker (%q) for %q: %v",
			brokerIDUsedToAuthenticate, user, err)
		if err := showPamMessage(mTx, pam.ErrorMsg, msg); err != nil {
			log.Warningf(context.TODO(), "Impossible to show PAM message: %v", err)
		}
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
func go_pam_cleanup_module() {}
