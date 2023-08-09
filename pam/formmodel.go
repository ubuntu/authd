package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type formModel struct {
	label string

	focusableModels []authorizationComponent
	focusIndex      int

	wait bool
}

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

func (m formModel) Init() tea.Cmd {
	return nil
}

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
				return m, sendEvent(isAuthorizedRequested{`{"wait": "true"}`})
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
		m.focusableModels[i] = newModel.(authorizationComponent)
	}

	return m, cmd
}

func (m formModel) View() string {
	fields := []string{m.label}

	for _, fm := range m.focusableModels {
		fields = append(fields, fm.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		fields...,
	)
}

func (m formModel) Focus() tea.Cmd {
	if m.focusIndex >= len(m.focusableModels) {
		return nil
	}
	return m.focusableModels[m.focusIndex].Focus()
}

func (m formModel) Blur() {
	if m.focusIndex >= len(m.focusableModels) {
		return
	}
	m.focusableModels[m.focusIndex].Blur()
}
