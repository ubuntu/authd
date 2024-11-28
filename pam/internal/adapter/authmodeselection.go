package adapter

import (
	"context"
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/brokers/layouts"
	"github.com/ubuntu/authd/brokers/layouts/entries"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/proto"
)

// authModeSelectionModel is the model list to select supported authentication modes.
type authModeSelectionModel struct {
	list.Model
	focused bool

	clientType                PamClientType
	supportedUILayouts        []*proto.UILayout
	availableAuthModes        []*proto.GAMResponse_AuthenticationMode
	currentAuthModeSelectedID string
}

// supportedUILayoutsReceived is the internal event signalling that the current supported ui layout in the context have been received.
type supportedUILayoutsReceived struct {
	layouts []*proto.UILayout
}

// supportedUILayoutsSet is the event signalling that the current supported ui layout in the context have been set.
type supportedUILayoutsSet struct{}

// authModesReceived is the internal event signalling that the supported authentication modes have been received.
type authModesReceived struct {
	authModes []*proto.GAMResponse_AuthenticationMode
}

// authModeSelected is the internal event signalling that the an authentication mode has been selected.
type authModeSelected struct {
	id string
}

// selectAuthMode selects current authentication mode.
func selectAuthMode(id string) tea.Cmd {
	return func() tea.Msg {
		return authModeSelected{
			id: id,
		}
	}
}

// newAuthModeSelectionModel initializes an empty list with default options of authModeSelectionModel.
func newAuthModeSelectionModel(clientType PamClientType) authModeSelectionModel {
	// FIXME: decouple UI from data model.
	if clientType != InteractiveTerminal {
		return authModeSelectionModel{
			Model:      list.New(nil, itemLayout{}, 0, 0),
			clientType: clientType,
		}
	}

	l := list.New(nil, itemLayout{}, 80, 24)
	l.Title = "Select your authentication method"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	l.Styles.Title = lipgloss.NewStyle()
	/*l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle*/

	return authModeSelectionModel{
		Model:      l,
		clientType: clientType,
	}
}

// Init initializes authModeSelectionModel.
func (m *authModeSelectionModel) Init() tea.Cmd {
	if m.clientType != InteractiveTerminal {
		// This is handled by the GDM or Native model!
		return nil
	}
	return func() tea.Msg {
		supportedEntries := layouts.OptionalItems(
			entries.Chars,
			entries.CharsPassword,
		)

		return supportedUILayoutsReceived{
			layouts: []*proto.UILayout{
				layouts.NewUI(layouts.UIForm,
					layouts.WithLabel(layouts.Required),
					layouts.WithEntry(supportedEntries),
					layouts.WithWait(layouts.OptionalWithBooleans),
					layouts.WithButton(layouts.Optional),
				).UILayout,
				layouts.NewUI(layouts.UIQrCode,
					layouts.WithContent(layouts.Required),
					layouts.WithCode(layouts.Optional),
					layouts.WithWait(layouts.RequiredWithBooleans),
					layouts.WithLabel(layouts.Optional),
					layouts.WithButton(layouts.Optional),
					layouts.WithRendersQrCode(true),
				).UILayout,
				layouts.NewUI(layouts.UINewPassword,
					layouts.WithLabel(layouts.Required),
					layouts.WithEntry(supportedEntries),
					layouts.WithButton(layouts.Optional),
				).UILayout,
			},
		}
	}
}

// Update handles events and actions.
func (m authModeSelectionModel) Update(msg tea.Msg) (authModeSelectionModel, tea.Cmd) {
	switch msg := msg.(type) {
	case supportedUILayoutsReceived:
		log.Debugf(context.TODO(), "%#v", msg)
		if len(msg.layouts) == 0 {
			return m, sendEvent(pamError{
				status: pam.ErrCredUnavail,
				msg:    "UI does not support any layouts",
			})
		}
		m.supportedUILayouts = msg.layouts
		return m, sendEvent(supportedUILayoutsSet{})

	case authModesReceived:
		log.Debugf(context.TODO(), "%#v", msg)
		m.availableAuthModes = msg.authModes

		var allAuthModes []list.Item
		var firstAuthModeID string
		for _, a := range m.availableAuthModes {
			if firstAuthModeID == "" {
				firstAuthModeID = a.Id
			}
			allAuthModes = append(allAuthModes, authModeItem{
				id:    a.Id,
				label: a.Label,
			})
		}

		cmds := []tea.Cmd{m.SetItems(allAuthModes)}
		// Autoselect first auth mode if any.
		if firstAuthModeID != "" {
			cmds = append(cmds, selectAuthMode(firstAuthModeID))
		}

		return m, tea.Sequence(cmds...)

	case authModeSelected:
		log.Debugf(context.TODO(), "%#v", msg)
		// Ensure auth mode id is valid
		if !validAuthModeID(msg.id, m.availableAuthModes) {
			log.Infof(context.TODO(), "authentication mode %q is not part of currently available authentication mode", msg.id)
			return m, nil
		}
		m.currentAuthModeSelectedID = msg.id

		// Select correct line to ensure model is synchronised
		for i, a := range m.Items() {
			a := convertTo[authModeItem](a)
			if a.id != msg.id {
				continue
			}
			m.Select(i)
		}

		return m, sendEvent(AuthModeSelected{
			ID: msg.id,
		})
	}

	if m.clientType != InteractiveTerminal {
		return m, nil
	}

	// interaction events
	if !m.focused {
		return m, nil
	}
	switch msg := msg.(type) {
	// Key presses
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			item := m.SelectedItem()
			if item == nil {
				return m, nil
			}
			authMode := convertTo[authModeItem](item)
			cmd := selectAuthMode(authMode.id)
			return m, cmd
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// This is necessarily an integer, so above
			choice, _ := strconv.Atoi(msg.String())
			items := m.Items()
			if choice > len(items) {
				return m, nil
			}
			item := items[choice-1]
			authMode := convertTo[authModeItem](item)
			cmd := selectAuthMode(authMode.id)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	return m, cmd
}

// Focus focuses this model.
func (m *authModeSelectionModel) Focus() tea.Cmd {
	m.focused = true
	return nil
}

// Focused returns if this model is focused.
func (m *authModeSelectionModel) Focused() bool {
	return m.focused
}

// Blur releases the focus from this model.
func (m *authModeSelectionModel) Blur() {
	m.focused = false
}

// WillCaptureEscape returns if this broker may capture Esc typing on keyboard.
func (m authModeSelectionModel) WillCaptureEscape() bool {
	return m.FilterState() == list.Filtering
}

// authModeItem is the list item corresponding to an authentication mode.
type authModeItem struct {
	id    string
	label string
}

// FilterValue allows filtering the list items.
func (i authModeItem) FilterValue() string { return "" }

// validAuthModeID returns if a authmode ID exists in the available list.
func validAuthModeID(id string, authModes []*proto.GAMResponse_AuthenticationMode) bool {
	for _, a := range authModes {
		if a.Id != id {
			continue
		}
		return true
	}
	return false
}

// getAuthenticationModes returns available authentication mode for this broker from authd.
func getAuthenticationModes(client proto.PAMClient, sessionID string, uiLayouts []*proto.UILayout) tea.Cmd {
	return func() tea.Msg {
		gamReq := &proto.GAMRequest{
			SessionId:          sessionID,
			SupportedUiLayouts: uiLayouts,
		}

		gamResp, err := client.GetAuthenticationModes(context.Background(), gamReq)
		if err != nil {
			return pamError{
				status: pam.ErrSystem,
				msg:    fmt.Sprintf("could not get authentication modes: %v", err),
			}
		}

		authModes := gamResp.GetAuthenticationModes()
		if len(authModes) == 0 {
			return pamError{
				status: pam.ErrCredUnavail,
				msg:    "no supported authentication mode available for this provider",
			}
		}
		log.Debug(context.TODO(), "authModes", authModes)

		return authModesReceived{
			authModes: authModes,
		}
	}
}

// Resets zeroes any internal state on the authModeSelectionModel.
func (m *authModeSelectionModel) Reset() {
	m.currentAuthModeSelectedID = ""
}

// SupportedUILayouts returns safely currently loaded supported ui layouts.
func (m authModeSelectionModel) SupportedUILayouts() []*proto.UILayout {
	return m.supportedUILayouts
}
