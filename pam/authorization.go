package main

import (
	"context"
	"encoding/json"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func sendIsAuthorized(ctx context.Context, client authd.PAMClient, sessionID, content string) tea.Cmd {
	return func() tea.Msg {
		res, err := client.IsAuthorized(ctx, &authd.IARequest{
			SessionId:          sessionID,
			AuthenticationData: content,
		})
		if err != nil {
			if st := status.Convert(err); st.Code() == codes.Canceled {
				return isAuthorizedResultReceived{
					access: "cancelled",
				}
			}
			return pamIgnore{
				msg: fmt.Sprintf("Authorisation status failure: %v", err),
			}
		}

		return isAuthorizedResultReceived{
			access: res.Access,
			data:   res.Data,
		}
	}
}

type isAuthorizedRequested struct {
	content string
}
type isAuthorizedResultReceived struct {
	access string
	data   string
}
type reselectAuthMode struct{}

type authorizationComponent interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (tea.Model, tea.Cmd)
	View() string
	Focus() tea.Cmd
	Blur()
}

type authorizationModel struct {
	focused bool

	client authd.PAMClient

	currentModel       authorizationComponent
	currentSessionID   string
	cancelIsAuthorized func()

	errorMsg string
}

type startAuthorization struct{}

func newAuthorizationModel(client authd.PAMClient) authorizationModel {
	return authorizationModel{
		client:             client,
		cancelIsAuthorized: func() {},
	}
}

func (m *authorizationModel) Init() tea.Cmd {
	return nil
}

func (m *authorizationModel) Update(msg tea.Msg) (authorizationModel, tea.Cmd) {
	switch msg := msg.(type) {

	case reselectAuthMode:
		m.cancelIsAuthorized()
		return *m, sendEvent(AuthModeSelected{})

	case isAuthorizedRequested:
		m.cancelIsAuthorized()
		ctx, cancel := context.WithCancel(context.Background())
		m.cancelIsAuthorized = cancel
		return *m, sendIsAuthorized(ctx, m.client, m.currentSessionID, msg.content)

	case isAuthorizedResultReceived:
		log.Infof(context.TODO(), "isAuthorizedResultReceived: %v", msg.access)
		switch msg.access {
		case "allowed":
			return *m, sendEvent(pamSuccess{})

		case "retry":
			m.errorMsg = dataToMsg(msg.data)
			return *m, sendEvent(startAuthorization{})

		case "denied":
			errMsg := "Access denied"
			if err := dataToMsg(msg.data); err != "" {
				errMsg = err
			}
			return *m, sendEvent(pamAuthError{msg: errMsg})

		case "next":
			return *m, sendEvent(GetAuthenticationModesRequested{})

		case "cancelled":
			// nothing to do
			return *m, nil
		}
	}

	// interaction events
	if !m.Focused() {
		return *m, nil
	}

	var cmd tea.Cmd
	var model tea.Model
	if m.currentModel != nil {
		model, cmd = m.currentModel.Update(msg)
		m.currentModel = model.(authorizationComponent)
	}
	return *m, cmd
}

func (m *authorizationModel) Focus() tea.Cmd {
	m.focused = true

	if m.currentModel == nil {
		return nil
	}
	return m.currentModel.Focus()
}

func (m *authorizationModel) Focused() bool {
	return m.focused
}

func (m *authorizationModel) Blur() {
	m.focused = false

	if m.currentModel == nil {
		return
	}
	m.currentModel.Blur()
}

func (m *authorizationModel) Compose(sessionID string, layout *authd.UILayout) tea.Cmd {
	m.currentSessionID = sessionID
	m.cancelIsAuthorized = func() {}

	switch layout.Type {
	case "form":
		var oldEntryValue string
		// We need to port previous entry after a reselection (indicated by the fact that we didn’t clear the previous model)
		if oldModel, ok := m.currentModel.(formModel); ok && layout.GetEntry() != "" {
			oldEntryValue = oldModel.getEntryValue()
		}
		form := newFormModel(layout.GetLabel(), layout.GetEntry(), layout.GetButton(), layout.GetWait() == "true")
		if oldEntryValue != "" {
			form.setEntryValue(oldEntryValue)
		}
		m.currentModel = form
	default:
		return sendEvent(pamSystemError{msg: fmt.Sprintf("unknown layout type: %q", layout.Type)})
	}

	return sendEvent(startAuthorization{})
}

func (m authorizationModel) View() string {
	if m.currentModel == nil {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.currentModel.View(),
		m.errorMsg,
	)
}

func (m *authorizationModel) Reset() {
	m.cancelIsAuthorized()
	m.cancelIsAuthorized = func() {}
	m.currentModel = nil
	m.currentSessionID = ""
}

func dataToMsg(data string) string {
	if data == "" {
		return ""
	}

	v := make(map[string]string)
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		log.Infof(context.TODO(), "Invalid json data from provider: %v", data)
		return ""
	}
	if len(v) == 0 {
		return ""
	}

	r, ok := v["message"]
	if !ok {
		log.Debugf(context.TODO(), "No message entry in json data from provider: %v", data)
		return ""
	}
	return r
}

func dataToUserInfo(data string) string {
	/*v := make(map[string]string)
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		log.Info(context.TODO(), "Invalid json data from provider: %v", data)
		return ""
	}*/

	return "TODO"
}
