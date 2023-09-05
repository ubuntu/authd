package testutils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const (
	objectPathFmt = "/com/ubuntu/authd/%s"
	interfaceFmt  = "com.ubuntu.authd.%s"

	// IDSeparator is the value used to append values to the sessionID in the broker mock.
	IDSeparator = "_separator_"
)

var brokerConfigTemplate = `name = %s
brand_icon = mock_icon.png

[dbus]
name = com.ubuntu.authd.%s
object = /com/ubuntu/authd/%s
interface = com.ubuntu.authd.%s
`

type isAuthorizedCtx struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// BrokerBusMock is the D-Bus object that will answer calls for the broker mock.
type BrokerBusMock struct {
	name                string
	isAuthorizedCalls   map[string]isAuthorizedCtx
	isAuthorizedCallsMu sync.RWMutex
}

// StartBusBrokerMock starts the D-Bus service and exports it on the system bus.
// It returns the configuration file path for the exported broker.
func StartBusBrokerMock(cfgDir string, brokerName string) (string, func(), error) {
	busObjectPath := fmt.Sprintf(objectPathFmt, brokerName)
	busInterface := fmt.Sprintf(interfaceFmt, brokerName)

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return "", nil, err
	}

	bus := BrokerBusMock{
		name:                brokerName,
		isAuthorizedCalls:   map[string]isAuthorizedCtx{},
		isAuthorizedCallsMu: sync.RWMutex{},
	}

	if err = conn.Export(&bus, dbus.ObjectPath(busObjectPath), busInterface); err != nil {
		conn.Close()
		return "", nil, err
	}

	err = conn.Export(introspect.NewIntrospectable(&introspect.Node{
		Name: busObjectPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{
				Name:    busInterface,
				Methods: introspect.Methods(&bus),
			},
		},
	}), dbus.ObjectPath(busObjectPath), introspect.IntrospectData.Name)
	if err != nil {
		conn.Close()
		return "", nil, err
	}

	reply, err := conn.RequestName(busInterface, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return "", nil, err
	}

	configPath, err := writeConfig(cfgDir, brokerName)
	if err != nil {
		conn.Close()
		return "", nil, err
	}

	return configPath, func() {
		_, _ = conn.ReleaseName(busInterface)
		_ = conn.Close()
	}, nil
}

func writeConfig(cfgDir, name string) (string, error) {
	cfgPath := filepath.Join(cfgDir, name)
	s := fmt.Sprintf(brokerConfigTemplate, name, name, name, name)
	if err := os.WriteFile(cfgPath, []byte(s), 0600); err != nil {
		return "", err
	}
	return cfgPath, nil
}

// NewSession returns default values to be used in tests or an error if requested.
func (b *BrokerBusMock) NewSession(username, lang string) (sessionID, encryptionKey string, dbusErr *dbus.Error) {
	parsedUsername := parseSessionID(username)
	if parsedUsername == "NS_error" {
		return "", "", dbus.MakeFailedError(fmt.Errorf("Broker %q: NewSession errored out", b.name))
	}
	if parsedUsername == "NS_no_id" {
		return "", username + "_key", nil
	}
	return fmt.Sprintf("%s-session_id", username), b.name + "_key", nil
}

// GetAuthenticationModes returns default values to be used in tests or an error if requested.
func (b *BrokerBusMock) GetAuthenticationModes(sessionID string, supportedUILayouts []map[string]string) (authenticationModes []map[string]string, dbusErr *dbus.Error) {
	sessionID = parseSessionID(sessionID)
	switch sessionID {
	case "GAM_invalid":
		return []map[string]string{
			{"invalid": "invalid"},
		}, nil
	case "GAM_empty":
		return nil, nil
	case "GAM_error":
		return nil, dbus.MakeFailedError(fmt.Errorf("Broker %q: GetAuthenticationModes errored out", b.name))
	case "GAM_multiple_modes":
		return []map[string]string{
			{"id": "mode1", "label": "Mode 1"},
			{"id": "mode2", "label": "Mode 2"},
		}, nil
	default:
		return []map[string]string{
			{"id": "mode1", "label": "Mode 1"},
		}, nil
	}
}

// SelectAuthenticationMode returns default values to be used in tests or an error if requested.
func (b *BrokerBusMock) SelectAuthenticationMode(sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, dbusErr *dbus.Error) {
	sessionID = parseSessionID(sessionID)
	switch sessionID {
	case "SAM_success_required_entry":
		return map[string]string{
			"type":  "required-entry",
			"entry": "entry_type",
		}, nil
	case "SAM_success_optional_entry":
		return map[string]string{
			"type":  "optional-entry",
			"entry": "entry_type",
		}, nil
	case "SAM_missing_optional_entry":
		return map[string]string{
			"type": "optional-entry",
		}, nil
	case "SAM_invalid_layout_type":
		return map[string]string{
			"invalid": "invalid",
		}, nil
	case "SAM_missing_required_entry":
		return map[string]string{
			"type": "required-entry",
		}, nil
	case "SAM_invalid_required_entry":
		return map[string]string{
			"type":  "required-entry",
			"entry": "invalid entry",
		}, nil
	case "SAM_invalid_optional_entry":
		return map[string]string{
			"type":  "optional-entry",
			"entry": "invalid entry",
		}, nil
	case "SAM_error":
		return nil, dbus.MakeFailedError(fmt.Errorf("Broker %q: SelectAuthenticationMode errored out", b.name))
	case "SAM_no_layout":
		return nil, nil
	case "SAM_empty_layout":
		return map[string]string{}, nil
	}
	// Should never get here
	return map[string]string{}, nil
}

// IsAuthorized returns default values to be used in tests or an error if requested.
func (b *BrokerBusMock) IsAuthorized(sessionID, authenticationData string) (access, data string, dbusErr *dbus.Error) {
	// The IsAuthorized needs to function a bit differently to still allow tests to be executed in parallel.
	// We have to use both the prefixed sessionID and the parsed one in order to differentiate between test cases.
	parsedID := parseSessionID(sessionID)

	if parsedID == "IA_error" {
		return "", "", dbus.MakeFailedError(fmt.Errorf("Broker %q: IsAuthorized errored out", b.name))
	}

	// Handles the context that will be assigned for the IsAuthorized handler
	b.isAuthorizedCallsMu.RLock()
	if _, exists := b.isAuthorizedCalls[sessionID]; exists {
		b.isAuthorizedCallsMu.RUnlock()
		return "", "", dbus.MakeFailedError(fmt.Errorf("Broker %q: IsAuthorized already running for session %q", b.name, sessionID))
	}
	b.isAuthorizedCallsMu.RUnlock()

	ctx, cancel := context.WithCancel(context.Background())
	b.isAuthorizedCallsMu.Lock()
	b.isAuthorizedCalls[sessionID] = isAuthorizedCtx{ctx, cancel}
	b.isAuthorizedCallsMu.Unlock()

	// Cleans the call after it's done
	defer func() {
		b.isAuthorizedCallsMu.Lock()
		delete(b.isAuthorizedCalls, sessionID)
		b.isAuthorizedCallsMu.Unlock()
	}()

	access = "allowed"
	data = `{"mock_answer": "authentication allowed by default"}`
	if parsedID == "IA_invalid" {
		access = "invalid"
	}

	done := make(chan struct{})
	go func() {
		switch parsedID {
		case "IA_timeout":
			time.Sleep(time.Second)
			access = "denied"
			data = `{"mock_answer": "denied by time out"}`
		case "IA_wait":
			<-ctx.Done()
			access = "cancelled"
			data = `{"mock_answer": "cancelled by user"}`
		case "IA_second_call":
			select {
			case <-ctx.Done():
				access = "cancelled"
				data = `{"mock_answer": "cancelled by user"}`
			case <-time.After(2 * time.Second):
				access = "allowed"
				data = `{"mock_answer": "authentication allowed by timeout"}`
			}
		}
		//TODO: Add cases for the new access types
		close(done)
	}()
	<-done

	if parsedID == "IA_invalid_data" {
		data = "invalid"
	} else if parsedID == "IA_empty_data" {
		data = ""
	}

	return access, data, nil
}

// EndSession returns default values to be used in tests or an error if requested.
func (b *BrokerBusMock) EndSession(sessionID string) (dbusErr *dbus.Error) {
	sessionID = parseSessionID(sessionID)
	if sessionID == "ES_error" {
		return dbus.MakeFailedError(fmt.Errorf("Broker %q: EndSession errored out", b.name))
	}
	return nil
}

// CancelIsAuthorized cancels an ongoing IsAuthorized call if it exists.
func (b *BrokerBusMock) CancelIsAuthorized(sessionID string) (dbusErr *dbus.Error) {
	b.isAuthorizedCallsMu.Lock()
	defer b.isAuthorizedCallsMu.Unlock()
	if _, exists := b.isAuthorizedCalls[sessionID]; !exists {
		return nil
	}
	b.isAuthorizedCalls[sessionID].cancelFunc()
	delete(b.isAuthorizedCalls, sessionID)
	return nil
}

// parseSessionID is wrapper around the sessionID to remove some values appended during the tests.
//
// The sessionID can have multiple values appended to differentiate between subtests and avoid concurrency conflicts,
// and only the last value (i.e. "..._separator_ID-session_id") will be considered.
func parseSessionID(sessionID string) string {
	cut := strings.Split(sessionID, IDSeparator)
	if len(cut) == 0 {
		return ""
	}
	return strings.TrimSuffix(cut[len(cut)-1], "-session_id")
}
