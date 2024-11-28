package adapter

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ubuntu/authd/brokers/layouts"
	"github.com/ubuntu/authd/brokers/layouts/entries"
	"github.com/ubuntu/authd/internal/proto"
)

// formModel is the form layout type to allow authentication and return a challenge.
type formModel struct {
	label string

	focusableModels []authenticationComponent
	focusIndex      int

	wait bool
}

// newFormModel initializes and return a new formModel.
func newFormModel(label, entryType, buttonLabel string, wait bool) formModel {
	var focusableModels []authenticationComponent

	// TODO: add digits and force validation.
	switch entryType {
	case entries.Chars, entries.CharsPassword:
		entry := newTextInputModel(entryType)
		focusableModels = append(focusableModels, &entry)
	}
	if buttonLabel != "" {
		button := &buttonModel{label: buttonLabel}
		focusableModels = append(focusableModels, button)
	}

	return formModel{
		label: label,
		wait:  wait,

		focusableModels: focusableModels,
	}
}

// Init initializes formModel.
func (m formModel) Init() tea.Cmd {
	return nil
}

// Update handles events and actions.
func (m formModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case startAuthentication:
		// Reset the entry.
		for _, fm := range m.focusableModels {
			switch entry := fm.(type) {
			case *textinputModel:
				entry.SetValue("")
			}
		}

		if !m.wait {
			return m, nil
		}
		return m, sendEvent(isAuthenticatedRequested{
			item: &proto.IARequest_AuthenticationData_Wait{Wait: layouts.True},
		})
	}

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
				return m, sendEvent(isAuthenticatedRequested{
					item: &proto.IARequest_AuthenticationData_Challenge{
						Challenge: entry.Value(),
					},
				})
			case *buttonModel:
				return m, sendEvent(reselectAuthMode{})
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
func (m formModel) View() string {
	var fields []string

	if m.label != "" {
		fields = append(fields, m.label)
	}

	for _, fm := range m.focusableModels {
		fields = append(fields, fm.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		fields...,
	)
}

// Focus focuses this model.
func (m formModel) Focus() tea.Cmd {
	if m.focusIndex >= len(m.focusableModels) {
		return nil
	}
	return m.focusableModels[m.focusIndex].Focus()
}

// Blur releases the focus from this model.
func (m formModel) Blur() {
	if m.focusIndex >= len(m.focusableModels) {
		return
	}
	m.focusableModels[m.focusIndex].Blur()
}
