package adapter

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/pam/internal/proto"
)

// userSelectionModel allows selecting from PAM or interactively an user.
type userSelectionModel struct {
	textinput.Model

	pamMTx     pam.ModuleTransaction
	clientType PamClientType
	enabled    bool
}

// userSelected events to report that a new username has been selected.
type userSelected struct {
	username string
}

// userRequired events to pick a new username.
type userRequired struct{}

// sendUserSelected sends the event to select a new username.
func sendUserSelected(username string) tea.Cmd {
	return func() tea.Msg {
		return userSelected{username}
	}
}

// newUserSelectionModel returns an initialized userSelectionModel.
func newUserSelectionModel(pamMTx pam.ModuleTransaction, clientType PamClientType) userSelectionModel {
	u := textinput.New()
	if clientType != InteractiveTerminal {
		// Cursor events are racy: https://github.com/charmbracelet/bubbletea/issues/909.
		// FIXME: Avoid initializing the text input Model at all.
		u.Cursor.SetMode(cursor.CursorHide)
	}
	u.Prompt = "Username: " // TODO: i18n
	u.Placeholder = "user name"

	//TODO: u.Validate
	return userSelectionModel{
		Model: u,

		pamMTx:     pamMTx,
		clientType: clientType,
	}
}

// Init initializes userSelectionModel, by getting it from PAM if prefilled.
func (m *userSelectionModel) Init() tea.Cmd {
	pamUser, err := m.pamMTx.GetItem(pam.User)
	if err != nil {
		return sendEvent(pamError{status: pam.ErrSystem, msg: err.Error()})
	}
	if pamUser != "" {
		return sendUserSelected(pamUser)
	}
	return sendEvent(userRequired{})
}

// Update handles events and actions.
func (m userSelectionModel) Update(msg tea.Msg) (userSelectionModel, tea.Cmd) {
	switch msg := msg.(type) {
	case userSelected:
		log.Debugf(context.TODO(), "%#v", msg)
		currentUser, err := m.pamMTx.GetItem(pam.User)
		if err != nil {
			return m, sendEvent(pamError{status: pam.ErrSystem, msg: err.Error()})
		}
		if !m.enabled && currentUser != "" && msg.username != currentUser {
			sendEvent(pamError{
				status: pam.ErrPermDenied,
				msg:    fmt.Sprintf("Changing username %s to %s is not allowed", m.Value(), msg.username),
			})
			return m, nil
		}
		if msg.username != "" && currentUser != msg.username {
			if err := m.pamMTx.SetItem(pam.User, msg.username); err != nil {
				return m, sendEvent(pamError{status: pam.ErrSystem, msg: err.Error()})
			}
		}
		if msg.username != "" {
			// synchronise our internal validated field and the text one.
			m.SetValue(msg.username)
			return m, sendEvent(UsernameOrBrokerListReceived{})
		}
		return m, nil

	case userRequired:
		m.enabled = true
		return m, sendEvent(ChangeStage{Stage: proto.Stage_userSelection})
	}

	if !m.enabled {
		return m, nil
	}

	if m.clientType != InteractiveTerminal {
		return m, nil
	}

	// interaction events
	if !m.Focused() {
		return m, nil
	}
	switch msg := msg.(type) {
	// Key presses
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			cmd := sendUserSelected(m.Value())
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	return m, cmd
}

// Enabled returns whether the interactive user selection is enabled.
func (m userSelectionModel) Enabled() bool {
	return m.enabled
}
