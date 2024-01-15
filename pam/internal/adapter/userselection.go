package adapter

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/msteinert/pam/v2"
)

// userSelectionModel allows selecting from PAM or interactively an user.
type userSelectionModel struct {
	textinput.Model

	pamMTx pam.ModuleTransaction
}

// userSelected events to select a new username.
type userSelected struct {
	username string
}

// sendUserSelected sends the event to select a new username.
func sendUserSelected(username string) tea.Cmd {
	return func() tea.Msg {
		return userSelected{username}
	}
}

// newUserSelectionModel returns an initialized userSelectionModel.
func newUserSelectionModel(pamMTx pam.ModuleTransaction) userSelectionModel {
	u := textinput.New()
	u.Prompt = "Username: " // TODO: i18n
	u.Placeholder = "user name"

	//TODO: u.Validate
	return userSelectionModel{
		Model: u,

		pamMTx: pamMTx,
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
	return nil
}

// Update handles events and actions.
func (m userSelectionModel) Update(msg tea.Msg) (userSelectionModel, tea.Cmd) {
	switch msg := msg.(type) {
	case userSelected:
		if msg.username != "" {
			// synchronise our internal validated field and the text one.
			m.SetValue(msg.username)
			if err := m.pamMTx.SetItem(pam.User, msg.username); err != nil {
				return m, sendEvent(pamError{status: pam.ErrAbort, msg: err.Error()})
			}
			return m, sendEvent(UsernameOrBrokerListReceived{})
		}
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
