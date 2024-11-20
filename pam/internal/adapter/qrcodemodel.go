package adapter

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/skip2/go-qrcode"
	"github.com/ubuntu/authd/internal/brokers/layouts"
	"github.com/ubuntu/authd/internal/proto/authd"
)

var centeredStyle = lipgloss.NewStyle().Align(lipgloss.Center, lipgloss.Top)

// qrcodeModel is the form layout type to allow authenticating and return a challenge.
type qrcodeModel struct {
	label       string
	buttonModel *authReselectButtonModel

	content string
	code    string
	qrCode  *qrcode.QRCode

	wait bool
}

// newQRCodeModel initializes and return a new qrcodeModel.
func newQRCodeModel(content, code, label, buttonLabel string, wait bool) (qrcodeModel, error) {
	var button *authReselectButtonModel
	if buttonLabel != "" {
		button = newAuthReselectionButtonModel(buttonLabel)
	}

	qrCode, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return qrcodeModel{}, fmt.Errorf("can't generate QR code: %v", err)
	}

	return qrcodeModel{
		label:       label,
		buttonModel: button,
		content:     content,
		code:        code,
		qrCode:      qrCode,
		wait:        wait,
	}, nil
}

// Init initializes qrcodeModel.
func (m qrcodeModel) Init() tea.Cmd {
	return m.buttonModel.Init()
}

// Update handles events and actions.
func (m qrcodeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case startAuthentication:
		if !m.wait {
			return m, nil
		}
		return m, sendEvent(isAuthenticatedRequested{
			item: &authd.IARequest_AuthenticationData_Wait{Wait: layouts.True},
		})
	}

	model, cmd := m.buttonModel.Update(msg)
	m.buttonModel = convertTo[*authReselectButtonModel](model)

	return m, cmd
}

func (m qrcodeModel) renderQrCode() (qr string) {
	defer func() { qr = strings.TrimRight(qr, "\n") }()

	if os.Getenv("XDG_SESSION_TYPE") == "tty" {
		return m.qrCode.ToString(false)
	}

	switch termenv.DefaultOutput().Profile {
	case termenv.ANSI, termenv.Ascii:
		// This applies to less smart terminals such as xterm, or in a multiplexer.
		return m.qrCode.ToString(false)
	default:
		return m.qrCode.ToSmallString(false)
	}
}

// View renders a text view of the form.
func (m qrcodeModel) View() string {
	fields := []string{}
	if m.label != "" {
		fields = append(fields, m.label, "")
	}

	qr := m.renderQrCode()
	fields = append(fields, qr)
	qrcodeWidth := lipgloss.Width(qr)

	style := centeredStyle.Width(qrcodeWidth)
	renderedContent := m.content
	if lipgloss.Width(m.content) < qrcodeWidth {
		renderedContent = style.Render(m.content)
	}
	fields = append(fields, renderedContent)

	if m.code != "" {
		fields = append(fields, style.Render(m.code))
	}

	if m.buttonModel != nil {
		fields = append(fields, style.Render(m.buttonModel.View()))
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
