package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// formModel is the form layout type to allow authorizing and return a challenge.
type formModel struct {
	label string

	focusableModels []authorizationComponent
	focusIndex      int

	wait bool
}

// newFormModel initializes and return a new formModel.
func newFormModel(label, entryType, buttonLabel string, wait bool) formModel {
	var focusableModels []authorizationComponent

	// TODO: add digits and force validation.
	switch entryType {
	case "chars":
		entry := &textinputModel{Model: textinput.New()}
		focusableModels = append(focusableModels, entry)
	case "chars_password":
		entry := &textinputModel{Model: textinput.New()}
		entry.EchoMode = textinput.EchoNone
		focusableModels = append(focusableModels, entry)
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
	case startAuthorization:
		if !m.wait {
			return m, nil
		}
		return m, sendEvent(isAuthorizedRequested{`{"wait": "true"}`})
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
				return m, sendEvent(isAuthorizedRequested{content: fmt.Sprintf(`{"challenge": "%s"}`, entry.Value())})
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
		var newModel tea.Model
		newModel, cmd = fm.Update(msg)
		// it should be a authorizationComponent, otherwise it’s a programming error.
		c, ok := newModel.(authorizationComponent)
		if !ok {
			panic(fmt.Sprintf("expected authorizationComponent, got %T", c))
		}
		m.focusableModels[i] = c
	}

	return m, cmd
}

// View renders a text view of the form.
func (m formModel) View() string {
	fields := []string{m.label}

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

// getEntryValue returns previous entry value, if any.
func (m formModel) getEntryValue() string {
	for _, entry := range m.focusableModels {
		entry, ok := entry.(*textinputModel)
		if !ok {
			continue
		}
		return entry.Value()
	}
	return ""
}

// setEntryValue reset the entry (if present) to a given value.
func (m *formModel) setEntryValue(value string) {
	for _, entry := range m.focusableModels {
		entry, ok := entry.(*textinputModel)
		if !ok {
			continue
		}
		entry.SetValue(value)
		return
	}
}
