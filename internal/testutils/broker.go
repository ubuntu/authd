package testutils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/stretchr/testify/require"
)

const (
	objectPathFmt = "/com/ubuntu/authd/%s"
	interfaceFmt  = "com.ubuntu.authd.%s"
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
func StartBusBrokerMock(t *testing.T) string {
	t.Helper()

	brokerName := strings.ReplaceAll(t.Name(), "/", "_")
	busObjectPath := fmt.Sprintf(objectPathFmt, brokerName)
	busInterface := fmt.Sprintf(interfaceFmt, brokerName)

	conn, err := dbus.ConnectSystemBus()
	require.NoError(t, err, "Setup: could not connect to system bus")
	t.Cleanup(func() { require.NoError(t, conn.Close(), "Teardown: could not close system bus connection") })

	bus := BrokerBusMock{
		name:                brokerName,
		isAuthorizedCalls:   map[string]isAuthorizedCtx{},
		isAuthorizedCallsMu: sync.RWMutex{},
	}
	err = conn.Export(&bus, dbus.ObjectPath(busObjectPath), busInterface)
	require.NoError(t, err, "Setup: could not export mock broker")

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
	require.NoError(t, err, "Setup: could not export mock broker introspection")

	reply, err := conn.RequestName(busInterface, dbus.NameFlagDoNotQueue)
	require.NoError(t, err, "Setup: could not request mock broker name")
	require.Equal(t, reply, dbus.RequestNameReplyPrimaryOwner, "Setup: mock broker name already taken")

	configPath := writeConfig(t, brokerName)

	t.Cleanup(func() {
		r, err := conn.ReleaseName(busInterface)
		require.NoError(t, err, "Teardown: could not release mock broker name")
		require.Equal(t, r, dbus.ReleaseNameReplyReleased, "Teardown: mock broker name not released")
	})

	return configPath
}

func writeConfig(t *testing.T, name string) string {
	t.Helper()

	cfgPath := filepath.Join(t.TempDir(), "broker-cfg")

	s := fmt.Sprintf(brokerConfigTemplate, name, name, name, name)
	err := os.WriteFile(cfgPath, []byte(s), 0600)
	require.NoError(t, err, "Setup: Failed to write broker config file")

	return cfgPath
}

// NewSession returns default values to be used in tests or an error if requested.
func (b *BrokerBusMock) NewSession(username, lang string) (sessionID, encryptionKey string, dbusErr *dbus.Error) {
	if username == "NS_error" {
		return "", "", dbus.MakeFailedError(fmt.Errorf("Broker %q: NewSession errored out", b.name))
	}
	if username == "NS_no_id" {
		return "", username + "_key", nil
	}
	return fmt.Sprintf("%s-%s_session_id", b.name, username), b.name + "_key", nil
}

// GetAuthenticationModes returns default values to be used in tests or an error if requested.
func (b *BrokerBusMock) GetAuthenticationModes(sessionID string, supportedUILayouts []map[string]string) (authenticationModes []map[string]string, dbusErr *dbus.Error) {
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
	switch sessionID {
	case "SAM_success_required_value":
		return map[string]string{
			"type":  "required-value",
			"value": "value_type",
		}, nil
	case "SAM_success_optional_value":
		return map[string]string{
			"type":  "optional-value",
			"value": "value_type",
		}, nil
	case "SAM_missing_optional_value":
		return map[string]string{
			"type": "optional-value",
		}, nil
	case "SAM_invalid_layout_type":
		return map[string]string{
			"invalid": "invalid",
		}, nil
	case "SAM_missing_required_value":
		return map[string]string{
			"type": "required-value",
		}, nil
	case "SAM_invalid_required_value":
		return map[string]string{
			"type":  "required-value",
			"value": "invalid value",
		}, nil
	case "SAM_invalid_optional_value":
		return map[string]string{
			"type":  "optional-value",
			"value": "invalid value",
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
	if sessionID == "IA_error" {
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
	if sessionID == "IA_invalid" {
		access = "invalid"
	}

	done := make(chan struct{})
	go func() {
		switch sessionID {
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

	if sessionID == "IA_invalid_data" {
		data = "invalid"
	} else if sessionID == "IA_empty_data" {
		data = ""
	}

	return access, data, nil
}

// EndSession returns default values to be used in tests or an error if requested.
func (b *BrokerBusMock) EndSession(sessionID string) (dbusErr *dbus.Error) {
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
