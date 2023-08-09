package main

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/log"
)

var debug string

// state represents the stage object
type stage int

const (
	// stageUserSelection is to select a user.
	stageUserSelection stage = iota
	// stageUserSelection is to select a broker.
	stageBrokerSelection
	// stageUserSelection is to select an authentication mode.
	stageAuthModeSelection
	// stageChallenge let's the user entering a challenge or waiting from authorization from the broker.
	stageChallenge
)

// sessionInfo contains the global broker session information.
type sessionInfo struct {
	sessionID     string
	encryptionKey string
}

// model is the global models orchestrator.
type model struct {
	pamh   pamHandle
	client authd.PAMClient

	height              int
	width               int
	interactiveTerminal bool

	currentSession *sessionInfo

	userSelectionModel     userSelectionModel
	brokerSelectionModel   brokerSelectionModel
	authModeSelectionModel authModeSelectionModel
	authorizationModel     authorizationModel

	exitMsg error
}

/* global events */

// UsernameOrBrokerListReceived is received either when the user name is filled (pam or manually) and we got the broker list.
type UsernameOrBrokerListReceived struct{}

// BrokerSelected signifies that the broker has been chosen.
type BrokerSelected struct {
	BrokerID string
}

// SessionStarted signals that we started a session with a given broker.
type SessionStarted struct {
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

// Init initializes the main model orchestrator.
func (m *model) Init() tea.Cmd {
	m.userSelectionModel = newUserSelectionModel(m.pamh)
	var cmds []tea.Cmd
	cmds = append(cmds, m.userSelectionModel.Init())

	m.brokerSelectionModel = newBrokerSelectionModel(m.client)
	cmds = append(cmds, m.brokerSelectionModel.Init())

	m.authModeSelectionModel = newAuthModeSelectionModel(m.client)
	cmds = append(cmds, m.authModeSelectionModel.Init())

	m.authorizationModel = newAuthorizationModel(m.client)
	cmds = append(cmds, m.authorizationModel.Init())

	cmds = append(cmds, m.changeStage(stageUserSelection))
	return tea.Batch(cmds...)
}

// Update handles events and actions to be done from the main model orchestrator.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	log.Debugf(context.TODO(), "%+v", msg)

	switch msg := msg.(type) {
	// Key presses
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, sendEvent(pamAbort{msg: "cancelled"})
		case "esc":
			if m.brokerSelectionModel.WillCaptureEscape() || m.authModeSelectionModel.WillCaptureEscape() {
				break
			}
			var cmd tea.Cmd
			switch m.currentStage() {
			case stageBrokerSelection:
				cmd = m.changeStage(stageUserSelection)
			case stageAuthModeSelection:
				cmd = m.changeStage(stageBrokerSelection)
			case stageChallenge:
				cmd = m.changeStage(stageAuthModeSelection)
			}
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		m.brokerSelectionModel.SetHeight(m.height - 3)
		m.brokerSelectionModel.SetWidth(m.width)

	// Exit cases
	case pamIgnore:
		m.exitMsg = msg
		return m, m.quit()
	case pamAbort:
		m.exitMsg = msg
		return m, m.quit()
	case pamSystemError:
		m.exitMsg = msg
		return m, m.quit()
	case pamAuthError:
		m.exitMsg = msg
		return m, m.quit()
	case pamSuccess:
		m.exitMsg = msg
		return m, m.quit()

	// Events
	case UsernameOrBrokerListReceived:
		if m.username() == "" {
			return m, nil
		}
		if m.availableBrokers() == nil {
			return m, nil
		}

		// Got user and brokers? Time to auto or manually select.
		return m, tea.Sequence(
			m.changeStage(stageBrokerSelection),
			m.brokerSelectionModel.AutoSelectForUser(m.username()))

	case BrokerSelected:
		return m, startBrokerSession(m.client, msg.BrokerID, m.username())

	case SessionStarted:
		m.currentSession = &sessionInfo{
			sessionID:     msg.sessionID,
			encryptionKey: msg.encryptionKey,
		}
		return m, sendEvent(GetAuthenticationModesRequested{})

	case GetAuthenticationModesRequested:
		return m, tea.Sequence(
			getAuthenticationModes(m.client, m.currentSession.sessionID),
			m.changeStage(stageAuthModeSelection),
		)

	case AuthModeSelected:
		// Reselection/reset of current authentication mode requested (button clicked for instance)
		if msg.ID == "" {
			msg.ID = m.authModeSelectionModel.currentAuthModeSelectedID
		}
		if msg.ID == "" {
			return m, sendEvent(pamSystemError{msg: "reselection of current auth mode without current ID"})
		}
		return m, getLayout(m.client, m.currentSession.sessionID, msg.ID)

	case UILayoutReceived:
		log.Info(context.TODO(), "UILayoutReceived")

		return m, tea.Sequence(
			m.authorizationModel.Compose(m.currentSession.sessionID, msg.layout),
			m.changeStage(stageChallenge))

	case SessionEnded:
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
	m.authorizationModel, cmd = m.authorizationModel.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders a text view of the whole UI.
func (m *model) View() string {
	var view strings.Builder

	log.Info(context.TODO(), m.currentStage())
	switch m.currentStage() {
	case stageUserSelection:
		view.WriteString(m.userSelectionModel.View())
	case stageBrokerSelection:
		view.WriteString(m.brokerSelectionModel.View())
	case stageAuthModeSelection:
		view.WriteString(m.authModeSelectionModel.View())
	case stageChallenge:
		view.WriteString(m.authorizationModel.View())
	default:
		view.WriteString("INVALID STAGE")
	}

	if debug != "" {
		view.WriteString(debug)
	}

	return view.String()
}

// currentStage returns our current stage step.
func (m *model) currentStage() stage {
	if m.userSelectionModel.Focused() {
		return stageUserSelection
	}
	if m.brokerSelectionModel.Focused() {
		return stageBrokerSelection
	}
	if m.authModeSelectionModel.Focused() {
		return stageAuthModeSelection
	}
	if m.authorizationModel.Focused() {
		return stageChallenge
	}
	return stageUserSelection
}

// changeStage returns a command acting to change the current stage and reset any previous views.
func (m *model) changeStage(s stage) tea.Cmd {
	switch s {
	case stageUserSelection:
		m.brokerSelectionModel.Blur()
		m.authModeSelectionModel.Blur()
		m.authorizationModel.Blur()

		return m.userSelectionModel.Focus()

	case stageBrokerSelection:
		m.userSelectionModel.Blur()
		m.authModeSelectionModel.Blur()
		m.authorizationModel.Blur()

		m.authModeSelectionModel.Reset()

		return tea.Sequence(endSession(m.client, m.currentSession), m.brokerSelectionModel.Focus())

	case stageAuthModeSelection:
		m.userSelectionModel.Blur()
		m.brokerSelectionModel.Blur()
		m.authorizationModel.Blur()

		m.authorizationModel.Reset()

		return m.authModeSelectionModel.Focus()

	case stageChallenge:
		m.userSelectionModel.Blur()
		m.brokerSelectionModel.Blur()
		m.authModeSelectionModel.Blur()

		return m.authorizationModel.Focus()
	}

	// TODO: error
	return nil
}

// username returns currently selected user name.
func (m model) username() string {
	return m.userSelectionModel.Value()
}

// availableBrokers returns currently available brokers.
func (m model) availableBrokers() []*authd.ABResponse_BrokerInfo {
	return m.brokerSelectionModel.availableBrokers
}
