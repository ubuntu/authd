package adapter

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// buttonModel creates a virtual button model which can be focused.
type buttonModel struct {
	label string

	focused bool
}

// Init initializes buttonModel.
func (b buttonModel) Init() tea.Cmd {
	return nil
}

// Update handles events and actions.
func (b *buttonModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return b, nil
}

var (
	buttonStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
			MarginLeft(2).
			MarginTop(1)

	activeButtonStyle = buttonStyle.Copy().
				Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#000000"}).
				Background(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"})
)

// View renders a text view of the button.
func (b buttonModel) View() string {
	content := fmt.Sprintf("[ %s ]", b.label)
	if b.focused {
		return activeButtonStyle.Render(content)
	}
	return buttonStyle.Render(content)
}

// Focus focuses this model.
func (b *buttonModel) Focus() tea.Cmd {
	b.focused = true
	return nil
}

// Blur releases the focus from this model.
func (b *buttonModel) Blur() {
	b.focused = false
}
