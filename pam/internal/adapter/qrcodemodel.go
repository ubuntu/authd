package adapter

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/skip2/go-qrcode"
	"github.com/ubuntu/authd"
)

// qrcodeModel is the form layout type to allow authenticating and return a challenge.
type qrcodeModel struct {
	label       string
	buttonModel *buttonModel

	content string
	qrCode  *qrcode.QRCode

	wait bool
}

// newQRCodeModel initializes and return a new qrcodeModel.
func newQRCodeModel(content, label, buttonLabel string, wait bool) (qrcodeModel, error) {
	var button *buttonModel
	if buttonLabel != "" {
		button = &buttonModel{label: buttonLabel}
	}

	qrCode, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return qrcodeModel{}, fmt.Errorf("can't generate QR code: %v", err)
	}

	return qrcodeModel{
		label:       label,
		buttonModel: button,
		content:     content,
		qrCode:      qrCode,
		wait:        wait,
	}, nil
}

// Init initializes qrcodeModel.
func (m qrcodeModel) Init() tea.Cmd {
	return nil
}

// Update handles events and actions.
func (m qrcodeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case startAuthentication:
		if !m.wait {
			return m, nil
		}
		return m, sendEvent(isAuthenticatedRequested{
			item: &authd.IARequest_AuthenticationData_Wait{Wait: "true"},
		})
	}

	switch msg := msg.(type) {
	// Key presses
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.buttonModel == nil {
				return m, nil
			}
			return m, sendEvent(reselectAuthMode{})
		}
	}

	model, cmd := m.buttonModel.Update(msg)
	m.buttonModel = convertTo[*buttonModel](model)

	return m, cmd
}

// View renders a text view of the form.
func (m qrcodeModel) View() string {
	fields := []string{}
	if m.label != "" {
		fields = append(fields, m.label, "")
	}

	fields = append(fields, m.qrCode.ToSmallString(false))

	if m.buttonModel != nil {
		fields = append(fields, m.buttonModel.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		fields...,
	)
}

// Focus focuses this model.
func (m qrcodeModel) Focus() tea.Cmd {
	if m.buttonModel == nil {
		return nil
	}
	return m.buttonModel.Focus()
}

// Blur releases the focus from this model.
func (m qrcodeModel) Blur() {
	if m.buttonModel == nil {
		return
	}
	m.buttonModel.Blur()
}
