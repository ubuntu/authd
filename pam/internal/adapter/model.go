// Package adapter is the package for the PAM library
package adapter

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/internal/consts"
	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/authd/pam/internal/proto"
	pam_proto "github.com/ubuntu/authd/pam/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// PamClientType indicates the type of the PAM client we're handling.
type PamClientType int

const (
	// Native indicates a PAM Client that is not supporting any special protocol.
	Native PamClientType = iota
	// InteractiveTerminal indicates an interactive terminal we can directly write our interface to.
	InteractiveTerminal
	// Gdm is a gnome-shell client via GDM display manager.
	Gdm
)

var debug string

// sessionInfo contains the global broker session information.
type sessionInfo struct {
	brokerID      string
	sessionID     string
	encryptionKey *rsa.PublicKey
}

// UIModel is the global models orchestrator.
type UIModel struct {
	// PamMTx is the [pam.ModuleTransaction] used to communicate with PAM.
	PamMTx pam.ModuleTransaction
	// Conn is the [grpc.ClientConn] opened with authd daemon.
	Conn *grpc.ClientConn
	// PamClientType is the kind of the PAM client we're handling.
	ClientType PamClientType
	// SessionMode is the mode of the session invoked by the module.
	SessionMode authd.SessionMode

	// client is the [authd.PAMClient] handle used to communicate with authd.
	client authd.PAMClient

	sessionStartingForBroker string
	currentSession           *sessionInfo

	healthCheckCancel      func()
	userSelectionModel     userSelectionModel
	brokerSelectionModel   brokerSelectionModel
	authModeSelectionModel authModeSelectionModel
	authenticationModel    authenticationModel
	gdmModel               gdmModel
	nativeModel            nativeModel

	exitStatus PamReturnStatus
}

/* global events */

// BrokerListReceived is received when we got the broker list.
type BrokerListReceived struct{}

// UsernameSelected is received when the user name is filled (from pam or manually).
type UsernameSelected struct{}

// BrokerSelected signifies that the broker has been chosen.
type BrokerSelected struct {
	BrokerID string
}

// SessionStarted signals that we started a session with a given broker.
type SessionStarted struct {
	brokerID      string
	sessionID     string
	encryptionKey string
}

// GetAuthenticationModesRequested signals that a model needs to get the broker authentication modes.
type GetAuthenticationModesRequested struct{}

// AuthModeSelected is triggered when the authentication mode has been chosen.
type AuthModeSelected struct {
	ID string
}

// UILayoutReceived means that we got the ui layout to display by the broker.
type UILayoutReceived struct {
	layout *authd.UILayout
}

// SessionEnded signals that the session is done and closed from the broker.
type SessionEnded struct{}

// ChangeStage signals that the model requires a stage change.
type ChangeStage struct {
	Stage pam_proto.Stage
}

// Init initializes the main model orchestrator.
func (m *UIModel) Init() tea.Cmd {
	var cmds []tea.Cmd

	if m.Conn != nil {
		m.client = authd.NewPAMClient(m.Conn)
	}

	switch m.ClientType {
	case Gdm:
		m.gdmModel = gdmModel{pamMTx: m.PamMTx}
		cmds = append(cmds, m.gdmModel.Init())
	case Native:
		var nssClient authd.NSSClient
		if m.Conn != nil && isSSHSession(m.PamMTx) {
			nssClient = authd.NewNSSClient(m.Conn)
		}
		m.nativeModel = nativeModel{pamMTx: m.PamMTx, nssClient: nssClient}
		cmds = append(cmds, m.nativeModel.Init())
	}

	if m.client == nil {
		return sendEvent(pamError{
			status: pam.ErrAbort,
			msg:    "No PAM client set",
		})
	}

	m.userSelectionModel = newUserSelectionModel(m.PamMTx, m.ClientType)
	cmds = append(cmds, m.userSelectionModel.Init())

	m.brokerSelectionModel = newBrokerSelectionModel(m.client, m.ClientType)
	cmds = append(cmds, m.brokerSelectionModel.Init())

	m.authModeSelectionModel = newAuthModeSelectionModel(m.ClientType)
	cmds = append(cmds, m.authModeSelectionModel.Init())

	m.authenticationModel = newAuthenticationModel(m.client, m.ClientType)
	cmds = append(cmds, m.authenticationModel.Init())

	m.healthCheckCancel = func() {}
	cmds = append(cmds, m.startHealthCheck())

	return tea.Batch(cmds...)
}

func (m *UIModel) startHealthCheck() tea.Cmd {
	if m.Conn == nil {
		return nil
	}

	var ctx context.Context
	ctx, m.healthCheckCancel = context.WithCancel(context.Background())
	healthClient := healthgrpc.NewHealthClient(m.Conn)
	hcReq := &healthgrpc.HealthCheckRequest{Service: consts.ServiceName}

	return func() tea.Msg {
		for {
			r, err := healthClient.Check(ctx, hcReq)
			if status.Convert(err).Code() == codes.Canceled {
				return nil
			}
			if err != nil {
				log.Errorf(ctx, "Health check failed: %v", err)
				// We just consider this as a serving failure, without writing the whole error.
				r = &healthgrpc.HealthCheckResponse{
					Status: healthgrpc.HealthCheckResponse_NOT_SERVING,
				}
			}
			if r.Status != healthgrpc.HealthCheckResponse_SERVING {
				return pamError{
					status: pam.ErrSystem,
					msg:    fmt.Sprintf("%s stopped serving", m.Conn.Target()),
				}
			}

			<-time.After(500 * time.Millisecond)
		}
	}
}

// Update handles events and actions to be done from the main model orchestrator.
func (m *UIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// Key presses
	case tea.KeyMsg:
		if log.IsLevelEnabled(log.DebugLevel) {
			debugKey := m.currentStage() != pam_proto.Stage_challenge
			switch msg.String() {
			case "enter", "ctrl+c", "esc", "tab", "shift+tab":
				debugKey = true
			}
			if debugKey {
				log.Debugf(context.TODO(), "Key: %q in stage %q", msg, m.currentStage())
			}
		}

		switch msg.String() {
		case "ctrl+c":
			return m, sendEvent(pamError{
				status: pam.ErrAbort,
				msg:    "cancel requested",
			})
		case "esc":
			if !m.canGoBack() {
				return m, nil
			}
			return m, sendEvent(ChangeStage{m.previousStage()})
		}

	// Exit cases
	case PamReturnStatus:
		log.Debugf(context.TODO(), "%#v", msg)
		if m.exitStatus != nil {
			// Nothing to do, we're already exiting...
			return m, nil
		}
		m.exitStatus = msg
		return m, m.quit()

	// Events
	case BrokerListReceived:
		log.Debugf(context.TODO(), "%#v, brokers: %#v", msg, m.availableBrokers())
		if m.availableBrokers() == nil {
			return m, nil
		}
		return m, m.userSelectionModel.SelectUser()

	case UsernameSelected:
		log.Debugf(context.TODO(), "%#v, user: %q", msg, m.username())
		if m.username() == "" {
			return m, nil
		}

		// Got user and brokers? Time to auto or manually select.
		return m, AutoSelectForUser(m.client, m.username())

	case BrokerSelected:
		log.Debugf(context.TODO(), "%#v", msg)
		if m.sessionStartingForBroker == "" {
			m.sessionStartingForBroker = msg.BrokerID
			return m, startBrokerSession(m.client, msg.BrokerID, m.username(), m.SessionMode)
		}
		if m.sessionStartingForBroker != msg.BrokerID {
			return m, tea.Sequence(endSession(m.client, m.currentSession), sendEvent(msg))
		}
	case SessionStarted:
		log.Debugf(context.TODO(), "%#v", msg)
		m.sessionStartingForBroker = ""
		pubASN1, err := base64.StdEncoding.DecodeString(msg.encryptionKey)
		if err != nil {
			return m, sendEvent(pamError{
				status: pam.ErrSystem,
				msg:    fmt.Sprintf("encryption key sent by broker is not a valid base64 encoded string: %v", err),
			})
		}

		pubKey, err := x509.ParsePKIXPublicKey(pubASN1)
		if err != nil {
			return m, sendEvent(pamError{
				status: pam.ErrSystem,
				msg:    fmt.Sprintf("encryption key send by broker is not valid: %v", err),
			})
		}
		rsaPublicKey, ok := pubKey.(*rsa.PublicKey)
		if !ok {
			return m, sendEvent(pamError{
				status: pam.ErrSystem,
				msg:    fmt.Sprintf("expected encryption key sent by broker to be  RSA public key, got %T", pubKey),
			})
		}

		m.currentSession = &sessionInfo{
			brokerID:      msg.brokerID,
			sessionID:     msg.sessionID,
			encryptionKey: rsaPublicKey,
		}
		return m, sendEvent(GetAuthenticationModesRequested{})

	case ChangeStage:
		log.Debugf(context.TODO(), "%#v", msg)
		return m, m.changeStage(msg.Stage)

	case GetAuthenticationModesRequested:
		log.Debugf(context.TODO(), "%#v", msg)
		if m.currentSession == nil {
			return m, nil
		}

		return m, tea.Sequence(
			getAuthenticationModes(m.client, m.currentSession.sessionID, m.authModeSelectionModel.SupportedUILayouts()),
			m.changeStage(pam_proto.Stage_authModeSelection),
		)

	case AuthModeSelected:
		log.Debugf(context.TODO(), "%#v", msg)
		if m.currentSession == nil {
			return m, nil
		}
		// Reselection/reset of current authentication mode requested (button clicked for instance)
		if msg.ID == "" {
			msg.ID = m.authModeSelectionModel.currentAuthModeSelectedID
		}
		if msg.ID == "" {
			return m, sendEvent(pamError{
				status: pam.ErrSystem,
				msg:    "reselection of current auth mode without current ID",
			})
		}
		return m, tea.Sequence(
			m.updateClientModel(msg),
			getLayout(m.client, m.currentSession.sessionID, msg.ID),
		)

	case UILayoutReceived:
		log.Debugf(context.TODO(), "%#v", msg)
		if m.currentSession == nil {
			return m, nil
		}

		return m, tea.Sequence(
			m.authenticationModel.Compose(
				m.currentSession.brokerID,
				m.currentSession.sessionID,
				m.currentSession.encryptionKey,
				msg.layout,
			),
			m.updateClientModel(msg),
		)

	case SessionEnded:
		log.Debugf(context.TODO(), "%#v", msg)
		m.sessionStartingForBroker = ""
		m.currentSession = nil
		return m, nil
	}

	var cmd tea.Cmd
	var cmds tea.BatchMsg
	m.userSelectionModel, cmd = m.userSelectionModel.Update(msg)
	cmds = append(cmds, cmd)
	m.brokerSelectionModel, cmd = m.brokerSelectionModel.Update(msg)
	cmds = append(cmds, cmd)
	m.authModeSelectionModel, cmd = m.authModeSelectionModel.Update(msg)
	cmds = append(cmds, cmd)
	m.authenticationModel, cmd = m.authenticationModel.Update(msg)
	cmds = append(cmds, cmd)

	cmds = append(cmds, m.updateClientModel(msg))

	return m, tea.Batch(cmds...)
}

func (m *UIModel) updateClientModel(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch m.ClientType {
	case Gdm:
		m.gdmModel, cmd = m.gdmModel.Update(msg)
	case Native:
		m.nativeModel, cmd = m.nativeModel.Update(msg)
	}
	return cmd
}

// View renders a text view of the whole UI.
func (m *UIModel) View() string {
	if m.ClientType != InteractiveTerminal {
		return ""
	}

	var view strings.Builder

	switch m.currentStage() {
	case pam_proto.Stage_userSelection:
		view.WriteString(m.userSelectionModel.View())
	case pam_proto.Stage_brokerSelection:
		view.WriteString(m.brokerSelectionModel.View())
	case pam_proto.Stage_authModeSelection:
		view.WriteString(m.authModeSelectionModel.View())
	case pam_proto.Stage_challenge:
		view.WriteString(m.authenticationModel.View())
	default:
		view.WriteString("INVALID STAGE")
	}

	if debug != "" {
		view.WriteString(debug)
	}

	return view.String()
}

// currentStage returns our current stage step.
func (m *UIModel) currentStage() pam_proto.Stage {
	if m.userSelectionModel.Focused() {
		return pam_proto.Stage_userSelection
	}
	if m.brokerSelectionModel.Focused() {
		return pam_proto.Stage_brokerSelection
	}
	if m.authModeSelectionModel.Focused() {
		return pam_proto.Stage_authModeSelection
	}
	if m.authenticationModel.Focused() {
		return pam_proto.Stage_challenge
	}
	return pam_proto.Stage_userSelection
}

// changeStage returns a command acting to change the current stage and reset any previous views.
func (m *UIModel) changeStage(s pam_proto.Stage) tea.Cmd {
	var commands []tea.Cmd
	if m.currentStage() != s {
		switch m.currentStage() {
		case pam_proto.Stage_userSelection:
			m.userSelectionModel.Blur()
		case pam_proto.Stage_brokerSelection:
			m.brokerSelectionModel.Blur()
		case pam_proto.Stage_authModeSelection:
			m.authModeSelectionModel.Blur()
		case pam_proto.Stage_challenge:
			m.authenticationModel.Blur()
			commands = append(commands, m.authenticationModel.Reset())
		}

		if m.ClientType == Gdm {
			commands = append(commands, m.gdmModel.changeStage(s))
		}

		if m.ClientType == Native {
			commands = append(commands, m.nativeModel.changeStage(s))
		}
	}

	switch s {
	case pam_proto.Stage_userSelection:
		// The session should be ended when going back to previous state, but we don’t quit the stage immediately
		// and so, we should always ensure we cancel previous session.
		commands = append(commands, endSession(m.client, m.currentSession), m.userSelectionModel.Focus())

	case pam_proto.Stage_brokerSelection:
		m.authModeSelectionModel.Reset()
		commands = append(commands, endSession(m.client, m.currentSession), m.brokerSelectionModel.Focus())

	case pam_proto.Stage_authModeSelection:
		commands = append(commands, m.authModeSelectionModel.Focus())

	case pam_proto.Stage_challenge:
		commands = append(commands, m.authenticationModel.Focus())

	default:
		return sendEvent(pamError{
			status: pam.ErrSystem,
			msg:    fmt.Sprintf("unknown PAM stage: %q", s),
		})
	}

	return tea.Sequence(commands...)
}

func (m UIModel) previousStage() pam_proto.Stage {
	currentStage := m.currentStage()
	if currentStage > proto.Stage_authModeSelection && len(m.availableAuthModes()) > 1 {
		return proto.Stage_authModeSelection
	}
	if currentStage > proto.Stage_brokerSelection && len(m.availableBrokers()) > 1 {
		return proto.Stage_brokerSelection
	}
	return proto.Stage_userSelection
}

func (m UIModel) canGoBack() bool {
	if m.userSelectionModel.Enabled() {
		return m.currentStage() > proto.Stage_userSelection
	}
	return m.previousStage() > proto.Stage_userSelection
}

// MsgFilter is the handler for the UI model.
func (m *UIModel) MsgFilter(model tea.Model, msg tea.Msg) tea.Msg {
	if m.ClientType != Gdm {
		return msg
	}

	if _, ok := msg.(tea.QuitMsg); ok {
		m.healthCheckCancel()
		m.gdmModel = m.gdmModel.stopConversations()
	}

	return msg
}

var errNoExitStatus = pamError{status: pam.ErrSystem, msg: "model did not return anything"}

// ExitStatus exposes the [PamReturnStatus] externally.
func (m *UIModel) ExitStatus() PamReturnStatus {
	if m.exitStatus == nil {
		return errNoExitStatus
	}
	return m.exitStatus
}

// username returns currently selected user name.
func (m UIModel) username() string {
	return m.userSelectionModel.Username()
}

// availableBrokers returns currently available brokers.
func (m UIModel) availableBrokers() []*authd.ABResponse_BrokerInfo {
	return m.brokerSelectionModel.availableBrokers
}

// availableBrokers returns currently available brokers.
func (m UIModel) availableAuthModes() []*authd.GAMResponse_AuthenticationMode {
	return m.authModeSelectionModel.availableAuthModes
}
