package adapter

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// textinputModel is a base block for handling textinput.Model, delegating to a tea.Model approach.
type textinputModel struct {
	textinput.Model
}

// Init initializes textinputModel.
func (m *textinputModel) Init() tea.Cmd {
	return nil
}

// Update handles events and actions.
func (m *textinputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	return m, cmd
}
