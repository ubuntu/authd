package main

import (
	"context"
	"encoding/json"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers/responses"
	"github.com/ubuntu/authd/internal/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sendIsAuthorized sends the authorization challenges or wait request to the brokers.
// The event will contain the returned value from the broker.
func sendIsAuthorized(ctx context.Context, client authd.PAMClient, sessionID, content string) tea.Cmd {
	return func() tea.Msg {
		res, err := client.IsAuthorized(ctx, &authd.IARequest{
			SessionId:          sessionID,
			AuthenticationData: content,
		})
		if err != nil {
			if st := status.Convert(err); st.Code() == codes.Canceled {
				return isAuthorizedResultReceived{
					access: responses.AuthCancelled,
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

// isAuthorizedRequested is the internal events signalling that authorization
// with the given challenge or wait has been requested.
type isAuthorizedRequested struct {
	content string
}

// isAuthorizedResultReceived is the internal event with the authorization access result
// and data that was retrieved.
type isAuthorizedResultReceived struct {
	access string
	data   string
}

// reselectAuthMode signals to restart auth mode selection with the same id (to resend sms or
// reenable the broker).
type reselectAuthMode struct{}

// authorizationComponent is the interface that all sub layout models needs to match.
type authorizationComponent interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (tea.Model, tea.Cmd)
	View() string
	Focus() tea.Cmd
	Blur()
}

// authorizationModel is the orchestrator model of all the authorization sub model layouts.
type authorizationModel struct {
	focused bool

	client authd.PAMClient

	currentModel       authorizationComponent
	currentSessionID   string
	currentBrokerID    string
	cancelIsAuthorized func()

	errorMsg string
}

// startAuthorization signals that the authorization model can start wait:true authorization if supported.
type startAuthorization struct{}

// newAuthorizationModel initializes a authorizationModel which needs to be Compose then.
func newAuthorizationModel(client authd.PAMClient) authorizationModel {
	return authorizationModel{
		client:             client,
		cancelIsAuthorized: func() {},
	}
}

// Init initializes authorizationModel.
func (m *authorizationModel) Init() tea.Cmd {
	return nil
}

// Update handles events and actions.
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
		case responses.AuthAllowed:
			return *m, sendEvent(pamSuccess{brokerID: m.currentBrokerID})

		case responses.AuthRetry:
			m.errorMsg = dataToMsg(msg.data)
			return *m, sendEvent(startAuthorization{})

		case responses.AuthDenied:
			errMsg := "Access denied"
			if err := dataToMsg(msg.data); err != "" {
				errMsg = err
			}
			return *m, sendEvent(pamAuthError{msg: errMsg})

		case responses.AuthNext:
			return *m, sendEvent(GetAuthenticationModesRequested{})

		case responses.AuthCancelled:
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
		m.currentModel = convertTo[authorizationComponent](model)
	}
	return *m, cmd
}

// Focus focuses this model.
func (m *authorizationModel) Focus() tea.Cmd {
	m.focused = true

	if m.currentModel == nil {
		return nil
	}
	return m.currentModel.Focus()
}

// Focused returns if this model is focused.
func (m *authorizationModel) Focused() bool {
	return m.focused
}

// Blur releases the focus from this model.
func (m *authorizationModel) Blur() {
	m.focused = false

	if m.currentModel == nil {
		return
	}
	m.currentModel.Blur()
}

// Compose creates and attaches the sub layout models based on UILayout.
func (m *authorizationModel) Compose(brokerID, sessionID string, layout *authd.UILayout) tea.Cmd {
	m.currentBrokerID = brokerID
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

	case "qrcode":
		qrcodeModel, err := newQRCodeModel(layout.GetContent(), layout.GetLabel(), layout.GetButton(), layout.GetWait() == "true")
		if err != nil {
			return sendEvent(pamSystemError{msg: err.Error()})
		}
		m.currentModel = qrcodeModel

	default:
		return sendEvent(pamSystemError{msg: fmt.Sprintf("unknown layout type: %q", layout.Type)})
	}

	return sendEvent(startAuthorization{})
}

// View renders a text view of the authorization UI.
func (m authorizationModel) View() string {
	if m.currentModel == nil {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.currentModel.View(),
		m.errorMsg,
	)
}

// Resets zeroes any internal state on the authorizationModel.
func (m *authorizationModel) Reset() {
	m.cancelIsAuthorized()
	m.cancelIsAuthorized = func() {}
	m.currentModel = nil
	m.currentSessionID = ""
	m.currentBrokerID = ""
}

// dataToMsg returns the data message from a given JSON message.
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

// dataToUserInfo returns the user information from a given JSON string.
//
//nolint:unused // This is not used for now TODO
func dataToUserInfo(data string) string {
	/*v := make(map[string]string)
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		log.Info(context.TODO(), "Invalid json data from provider: %v", data)
		return ""
	}*/

	return "TODO"
}
