package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type userSelectionModel struct {
	textinput.Model

	pamh pamHandle
}

type userSelected struct {
	username string
}

func sendUserSelected(username string) tea.Cmd {
	return func() tea.Msg {
		return userSelected{username}
	}
}

func newUserSelectionModel(pamh pamHandle) userSelectionModel {
	u := textinput.New()
	u.Prompt = "Username: " // TODO: i18n
	u.Placeholder = "user name"

	//TODO: u.Validate
	return userSelectionModel{
		Model: u,

		pamh: pamh,
	}
}

func (m *userSelectionModel) Init() tea.Cmd {
	pamUser := "user1" // TODO: remove once on pam
	//pamUser := getPAMUser(m.pamh)
	if pamUser != "" {
		return sendUserSelected(pamUser)
	}
	return nil
}

func (m userSelectionModel) Update(msg tea.Msg) (userSelectionModel, tea.Cmd) {
	switch msg := msg.(type) {
	case userSelected:
		if msg.username != "" {
			// synchronise our internal validated field and the text one.
			m.SetValue(msg.username)
			setPAMUser(m.pamh, msg.username)
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
