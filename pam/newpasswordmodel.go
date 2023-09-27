package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
)

// newPasswordModel is the form layout type to allow authentication and return a challenge.
type newPasswordModel struct {
	errorMsg string
	label    string

	passwordEntries []*textinputModel
	focusableModels []authenticationComponent
	focusIndex      int
}

// newNewPasswordModel initializes and return a new newPasswordModel.
func newNewPasswordModel(label, entryType, buttonLabel string) newPasswordModel {
	var focusableModels []authenticationComponent
	var passwordEntries []*textinputModel

	// TODO: add digits and force validation.
	for range []int{0, 1} {
		switch entryType {
		case "chars":
			entry := &textinputModel{Model: textinput.New()}
			passwordEntries = append(passwordEntries, entry)
			focusableModels = append(focusableModels, entry)
		case "chars_password":
			entry := &textinputModel{Model: textinput.New()}
			passwordEntries = append(passwordEntries, entry)
			entry.EchoMode = textinput.EchoNone
			focusableModels = append(focusableModels, entry)
		}
	}

	if buttonLabel != "" {
		button := &buttonModel{label: buttonLabel}
		focusableModels = append(focusableModels, button)
	}

	return newPasswordModel{
		label: label,

		passwordEntries: passwordEntries,
		focusableModels: focusableModels,
	}
}

// Init initializes newPasswordModel.
func (m newPasswordModel) Init() tea.Cmd {
	return nil
}

// Update handles events and actions.
func (m newPasswordModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// Key presses
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.focusIndex >= len(m.focusableModels) {
				return m, nil
			}
			entry := m.focusableModels[m.focusIndex]
			switch entry := entry.(type) {
			case *textinputModel:
				// Check both entries are matching
				if m.passwordEntries[0].Value() != m.passwordEntries[1].Value() {
					m.errorMsg = "Password entries don't match"
					return m, nil
				}

				m.errorMsg = ""
				return m, sendEvent(isAuthenticatedRequested{content: fmt.Sprintf(`{"challenge": "%s"}`, entry.Value())})

			case *buttonModel:
				return m, sendEvent(isAuthenticatedRequested{`{"skip": "true"}`})
			}

			return m, nil

		case "tab":
			m.focusIndex++
			if m.focusIndex == len(m.focusableModels) {
				m.focusIndex = 0
			}
			var cmd tea.Cmd
			for i, fm := range m.focusableModels {
				if i != m.focusIndex {
					fm.Blur()
					continue
				}
				cmd = fm.Focus()
			}
			return m, cmd

		default:
			m.errorMsg = ""
		}
	}

	var cmd tea.Cmd
	for i, fm := range m.focusableModels {
		if i != m.focusIndex {
			continue
		}
		var model tea.Model
		model, cmd = fm.Update(msg)
		m.focusableModels[i] = convertTo[authenticationComponent](model)
	}

	return m, cmd
}

// View renders a text view of the form.
func (m newPasswordModel) View() string {
	var fields []string
	if m.errorMsg != "" {
		fields = append(fields, errorStyle.Render(m.errorMsg))
	}
	fields = append(fields, m.label)

	for _, fm := range m.focusableModels {
		fields = append(fields, fm.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		fields...,
	)
}

// Focus focuses this model.
func (m newPasswordModel) Focus() tea.Cmd {
	if m.focusIndex >= len(m.focusableModels) {
		return nil
	}
	return m.focusableModels[m.focusIndex].Focus()
}

// Blur releases the focus from this model.
func (m newPasswordModel) Blur() {
	if m.focusIndex >= len(m.focusableModels) {
		return
	}
	m.focusableModels[m.focusIndex].Blur()
}
