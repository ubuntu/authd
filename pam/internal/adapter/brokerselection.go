package adapter

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/log"
)

// brokerSelectionModel is the model list selection layout to allow authenticating and return a challenge.
type brokerSelectionModel struct {
	list.Model
	focused bool

	client     authd.PAMClient
	clientType PamClientType

	availableBrokers []*authd.ABResponse_BrokerInfo
}

// brokersListReceived signals that the broker list from authd has been received.
type brokersListReceived struct {
	brokers []*authd.ABResponse_BrokerInfo
}

// brokerSelected is the internal event that a broker has been selected.
type brokerSelected struct {
	brokerID string
}

// selectBroker selects a given broker.
func selectBroker(brokerID string) tea.Cmd {
	return func() tea.Msg {
		return brokerSelected{
			brokerID: brokerID,
		}
	}
}

// newBrokerSelectionModel initializes an empty list with default options of brokerSelectionModel.
func newBrokerSelectionModel(client authd.PAMClient, clientType PamClientType) brokerSelectionModel {
	l := list.New(nil, itemLayout{}, 80, 24)
	l.Title = "Select your provider"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	l.Styles.Title = lipgloss.NewStyle()
	/*l.Styles.PaginationStyle = paginationStyle
	l.Styles.HelpStyle = helpStyle*/

	return brokerSelectionModel{
		Model:      l,
		client:     client,
		clientType: clientType,
	}
}

// Init initializes brokerSelectionModel by requesting the available brokers.
func (m brokerSelectionModel) Init() tea.Cmd {
	return getAvailableBrokers(m.client)
}

// Update handles events and actions.
func (m brokerSelectionModel) Update(msg tea.Msg) (brokerSelectionModel, tea.Cmd) {
	switch msg := msg.(type) {
	case brokersListReceived:
		log.Debugf(context.TODO(), "%#v", msg)
		if len(msg.brokers) == 0 {
			return m, sendEvent(pamError{
				status: pam.ErrAuthinfoUnavail,
				msg:    "No brokers available",
			})
		}
		m.availableBrokers = msg.brokers

		var allBrokers []list.Item
		for _, b := range m.availableBrokers {
			allBrokers = append(allBrokers, brokerItem{
				id:   b.Id,
				name: b.Name,
			})
		}
		var cmds []tea.Cmd
		cmds = append(cmds, m.SetItems(allBrokers))
		cmds = append(cmds, sendEvent(UsernameOrBrokerListReceived{}))

		return m, tea.Batch(cmds...)

	case brokerSelected:
		log.Debugf(context.TODO(), "%#v", msg)
		broker := brokerFromID(msg.brokerID, m.availableBrokers)
		if broker == nil {
			log.Infof(context.TODO(), "broker %q is not part of current active brokers", msg.brokerID)
			return m, nil
		}
		// Select correct line to ensure model is synchronised
		for i, b := range m.Items() {
			b := convertTo[brokerItem](b)
			if b.id != broker.Id {
				continue
			}
			m.Select(i)
		}

		return m, sendEvent(BrokerSelected{
			BrokerID: broker.Id,
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
			broker := convertTo[brokerItem](item)
			cmd := selectBroker(broker.id)
			return m, cmd
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// This is necessarily an integer, so above
			choice, _ := strconv.Atoi(msg.String())
			items := m.Items()
			if choice > len(items) {
				return m, nil
			}
			item := items[choice-1]
			broker := convertTo[brokerItem](item)
			cmd := selectBroker(broker.id)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	return m, cmd
}

// Focus focuses this model. It always returns nil.
func (m *brokerSelectionModel) Focus() tea.Cmd {
	m.focused = true
	return nil
}

// Focused returns if this model is focused.
func (m *brokerSelectionModel) Focused() bool {
	return m.focused
}

// Blur releases the focus from this model.
func (m *brokerSelectionModel) Blur() {
	m.focused = false
}

// AutoSelectForUser requests if any previous broker was used by this user to automatically selects it.
func AutoSelectForUser(client authd.PAMClient, username string) tea.Cmd {
	return func() tea.Msg {
		r, err := client.GetPreviousBroker(context.TODO(),
			&authd.GPBRequest{
				Username: username,
			})
		// We keep a chance to manually select the broker, not a blocker issue.
		if err != nil {
			log.Infof(context.TODO(), "can't get previous broker for %q", username)
			return nil
		}
		brokerID := r.GetPreviousBroker()
		if brokerID == "" {
			return nil
		}

		return selectBroker(brokerID)()
	}
}

// WillCaptureEscape returns if this broker may capture Esc typing on keyboard.
func (m brokerSelectionModel) WillCaptureEscape() bool {
	return m.FilterState() == list.Filtering
}

// brokerItem is the list item corresponding to a broker.
type brokerItem struct {
	id   string
	name string
}

// FilterValue allows filtering the list items.
func (i brokerItem) FilterValue() string { return "" }

// itemLayout is the rendering delegatation of brokerItem and authModeItem.
type itemLayout struct{}

// Height returns height of the items.
func (d itemLayout) Height() int { return 1 }

// Spacing returns the spacing needed between the items.
func (d itemLayout) Spacing() int { return 0 }

// Update triggers the update of each item.
func (d itemLayout) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// Render writes to w the rendering of the items based on its selection and type.
func (d itemLayout) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var label string
	switch item := item.(type) {
	case brokerItem:
		label = item.name
	case authModeItem:
		label = item.label
	default:
		log.Errorf(context.TODO(), "Unexpected item element type: %t", item)
		return
	}

	line := fmt.Sprintf("%d. %s", index+1, label)

	if index == m.Index() {
		line = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).Render("> " + line)
	} else {
		line = lipgloss.NewStyle().PaddingLeft(2).Render(line)
	}
	fmt.Fprint(w, line)
}

// getAvailableBrokers returns available broker list from authd.
func getAvailableBrokers(client authd.PAMClient) tea.Cmd {
	return func() tea.Msg {
		brokersInfo, err := client.AvailableBrokers(context.TODO(), &authd.Empty{})
		if err != nil {
			return pamError{
				status: pam.ErrSystem,
				msg:    fmt.Sprintf("could not get current available brokers: %v", err),
			}
		}

		return brokersListReceived{
			brokers: brokersInfo.BrokersInfos,
		}
	}
}

// brokerFromID return a broker matching brokerID if available, nil otherwise.
func brokerFromID(brokerID string, brokers []*authd.ABResponse_BrokerInfo) *authd.ABResponse_BrokerInfo {
	if brokerID == "" {
		return nil
	}

	for _, b := range brokers {
		if b.Id != brokerID {
			continue
		}
		return b
	}
	return nil
}
