package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type buttonModel struct {
	label string

	focused bool
}

func (b buttonModel) Init() tea.Cmd {
	return nil
}

func (b *buttonModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return b, nil
}

var (
	buttonStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
			MarginLeft(2).
			MarginTop(1)

	activeButtonStyle = buttonStyle.Copy().
				Background(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
				Underline(true)
)

func (b buttonModel) View() string {
	content := fmt.Sprintf("[ %s ]", b.label)
	if b.focused {
		return activeButtonStyle.Render(content)
	}
	return buttonStyle.Render(content)
}

func (b *buttonModel) Focus() tea.Cmd {
	b.focused = true
	return nil
}

func (b *buttonModel) Blur() {
	b.focused = false
}
