package adapter

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers/layouts/entries"
)

// newPasswordModel is the form layout type to allow authentication and return a challenge.
type newPasswordModel struct {
	errorMsg  string
	label     string
	skippable bool

	passwordEntries []*textinputModel
	passwordLabels  []string
	focusableModels []authenticationComponent
	focusIndex      int
}

// newNewPasswordModel initializes and return a new newPasswordModel.
func newNewPasswordModel(label, entryType, buttonLabel string) newPasswordModel {
	var focusableModels []authenticationComponent
	var passwordEntries []*textinputModel
	var skippable bool

	// TODO: add digits and force validation.
	for range []int{0, 1} {
		entry := &textinputModel{Model: textinput.New()}
		if entryType == entries.CharsPassword {
			entry.EchoMode = textinput.EchoPassword
		}
		passwordEntries = append(passwordEntries, entry)
		focusableModels = append(focusableModels, entry)
	}

	if buttonLabel != "" {
		skippable = true
		button := &buttonModel{label: buttonLabel}
		focusableModels = append(focusableModels, button)
	}

	return newPasswordModel{
		label:     label,
		skippable: skippable,

		passwordEntries: passwordEntries,
		passwordLabels:  []string{"New password:", "Confirm password:"},
		focusableModels: focusableModels,
	}
}

// Init initializes newPasswordModel.
func (m newPasswordModel) Init() tea.Cmd {
	var commands []tea.Cmd
	for _, c := range m.focusableModels {
		commands = append(commands, c.Init())
	}
	for _, c := range m.passwordEntries {
		commands = append(commands, c.Init())
	}
	return tea.Batch(commands...)
}

// Update handles events and actions.
func (m newPasswordModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case startAuthentication:
		m.Clear()
		return m, m.updateFocusModel(msg)

	case newPasswordCheckResult:
		if msg.msg != "" {
			m.Clear()
			return m, sendEvent(errMsgToDisplay{msg: msg.msg})
		}

		return m, tea.Batch(sendEvent(errMsgToDisplay{}), m.focusNextField())

	case buttonSelectionEvent:
		if m.focusIndex < len(m.focusableModels) &&
			msg.model == m.focusableModels[m.focusIndex] {
			return m, sendEvent(isAuthenticatedRequested{
				item: &authd.IARequest_AuthenticationData_Skip{Skip: "true"},
			})
		}

	case tea.KeyMsg: // Key presses
		switch msg.String() {
		case "tab", "shift+tab":
			// Only allow tabbing if the form is skippable
			if !m.skippable {
				return m, nil
			}

			m.errorMsg = ""

			// Only allow tabbing if no password was entered
			for _, pe := range m.passwordEntries {
				if pe.Value() != "" {
					return m, nil
				}
			}

			if m.focusIndex == 0 {
				return m, m.focusPrevField()
			}
			return m, m.focusNextField()

		case "enter":
			entry := m.focusableModels[m.focusIndex]
			switch entry := entry.(type) {
			case *textinputModel:
				m.errorMsg = ""

				// First entry is focused
				if m.focusIndex == 0 {
					// Check password quality
					return m, sendEvent(newPasswordCheck{challenge: m.passwordEntries[0].Value()})
				}

				// Second entry is focused
				if m.focusIndex == 1 {
					// Check both entries are matching
					if m.passwordEntries[0].Value() != m.passwordEntries[1].Value() {
						m.Clear()
						return m, sendEvent(errMsgToDisplay{msg: "Password entries don't match"})
					}
				}

				return m, sendEvent(isAuthenticatedRequested{
					item: &authd.IARequest_AuthenticationData_Challenge{Challenge: entry.Value()},
				})
			}

		default:
			m.errorMsg = ""
		}
	}

	return m, m.updateFocusModel(msg)
}

func (m *newPasswordModel) updateFocusModel(msg tea.Msg) tea.Cmd {
	if m.focusIndex >= len(m.focusableModels) {
		return nil
	}
	model, cmd := m.focusableModels[m.focusIndex].Update(msg)
	m.focusableModels[m.focusIndex] = convertTo[authenticationComponent](model)

	return cmd
}

// View renders a text view of the form.
func (m newPasswordModel) View() string {
	fields := []string{m.label, ""}

	for i, fm := range m.focusableModels {
		switch entry := fm.(type) {
		case *textinputModel:
			// Only show the second password entry if the first one is filled
			// (in case the form is advanced using Tab)
			if m.focusIndex == 1 && m.passwordEntries[0].Value() != "" || i == 0 {
				fields = append(fields, []string{m.passwordLabels[i], entry.View()}...)
			}
		case *buttonModel:
			fields = append(fields, entry.View())
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, fields...)
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

func (m *newPasswordModel) focusField(increment int) tea.Cmd {
	var cmd tea.Cmd
	focusableLen := len(m.focusableModels)

	// Wrap around
	m.focusIndex += increment
	if m.focusIndex < 0 || m.focusIndex >= focusableLen {
		m.focusIndex = (m.focusIndex%focusableLen + focusableLen) % focusableLen
	}

	for i, fm := range m.focusableModels {
		if i != m.focusIndex || cmd != nil {
			fm.Blur()
			continue
		}
		cmd = fm.Focus()
	}
	return cmd
}

func (m *newPasswordModel) focusNextField() tea.Cmd {
	return m.focusField(1)
}

func (m *newPasswordModel) focusPrevField() tea.Cmd {
	return m.focusField(-1)
}

func (m *newPasswordModel) Clear() {
	m.focusIndex = 0
	for i, fm := range m.focusableModels {
		switch entry := fm.(type) {
		case *textinputModel:
			entry.SetValue("")
		}
		if i != m.focusIndex {
			fm.Blur()
			continue
		}
		fm.Focus()
	}
}
