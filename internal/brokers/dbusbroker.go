package brokers

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/decorate"
	"gopkg.in/ini.v1"
)

// DbusInterface is the expected interface that should be implemented by the brokers.
const DbusInterface string = "com.ubuntu.authd.Broker"

type dbusBroker struct {
	dbusObject dbus.BusObject
}

// newDbusBroker returns a dbus broker and broker attributes from its configuration file.
func newDbusBroker(ctx context.Context, bus *dbus.Conn, configFile string) (b dbusBroker, fullName, brandIcon string, err error) {
	defer decorate.OnError(&err, "dbus broker from configuration file: %q", configFile)

	log.Debugf(ctx, "Dbus broker configuration at %q", configFile)

	cfg, err := ini.Load(configFile)
	if err != nil {
		return b, "", "", fmt.Errorf("could not read ini configuration for broker %v", err)
	}

	fullNameVal, err := cfg.Section("authd").GetKey("name")
	if err != nil {
		return b, "", "", fmt.Errorf("missing field for broker: %v", err)
	}

	brandIconVal, err := cfg.Section("authd").GetKey("brand_icon")
	if err != nil {
		return b, "", "", fmt.Errorf("missing field for broker: %v", err)
	}

	dbusName, err := cfg.Section("authd").GetKey("dbus_name")
	if err != nil {
		return b, "", "", fmt.Errorf("missing field for broker: %v", err)
	}

	objectName, err := cfg.Section("authd").GetKey("dbus_object")
	if err != nil {
		return b, "", "", fmt.Errorf("missing field for broker: %v", err)
	}

	return dbusBroker{
		dbusObject: bus.Object(dbusName.String(), dbus.ObjectPath(objectName.String())),
	}, fullNameVal.String(), brandIconVal.String(), nil
}

// NewSession calls the corresponding method on the broker bus and returns the session ID and encryption key.
func (b dbusBroker) NewSession(ctx context.Context, username, lang, mode string) (sessionID, encryptionKey string, err error) {
	dbusMethod := DbusInterface + ".NewSession"

	call := b.dbusObject.CallWithContext(ctx, dbusMethod, 0, username, lang, mode)
	if err = call.Err; err != nil {
		return "", "", err
	}
	if err = call.Store(&sessionID, &encryptionKey); err != nil {
		return "", "", err
	}

	return sessionID, encryptionKey, nil
}

// GetAuthenticationModes calls the corresponding method on the broker bus and returns the authentication modes supported by it.
func (b dbusBroker) GetAuthenticationModes(ctx context.Context, sessionID string, supportedUILayouts []map[string]string) (authenticationModes []map[string]string, err error) {
	dbusMethod := DbusInterface + ".GetAuthenticationModes"

	call := b.dbusObject.CallWithContext(ctx, dbusMethod, 0, sessionID, supportedUILayouts)
	if err = call.Err; err != nil {
		return nil, err
	}
	if err = call.Store(&authenticationModes); err != nil {
		return nil, err
	}

	return authenticationModes, nil
}

// SelectAuthenticationMode calls the corresponding method on the broker bus and returns the UI layout for the selected mode.
func (b dbusBroker) SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error) {
	dbusMethod := DbusInterface + ".SelectAuthenticationMode"

	call := b.dbusObject.CallWithContext(ctx, dbusMethod, 0, sessionID, authenticationModeName)
	if err = call.Err; err != nil {
		return nil, err
	}
	if err = call.Store(&uiLayoutInfo); err != nil {
		return nil, err
	}

	return uiLayoutInfo, nil
}

// IsAuthenticated calls the corresponding method on the broker bus and returns the user information and access.
func (b dbusBroker) IsAuthenticated(_ context.Context, sessionID, authenticationData string) (access, data string, err error) {
	dbusMethod := DbusInterface + ".IsAuthenticated"

	call := b.dbusObject.Call(dbusMethod, 0, sessionID, authenticationData)
	if err = call.Err; err != nil {
		return "", "", err
	}
	if err = call.Store(&access, &data); err != nil {
		return "", "", err
	}

	return access, data, nil
}

// EndSession calls the corresponding method on the broker bus.
func (b dbusBroker) EndSession(ctx context.Context, sessionID string) (err error) {
	dbusMethod := DbusInterface + ".EndSession"

	call := b.dbusObject.CallWithContext(ctx, dbusMethod, 0, sessionID)
	if err = call.Err; err != nil {
		return err
	}

	return nil
}

// CancelIsAuthenticated calls the corresponding method on the broker bus.
func (b dbusBroker) CancelIsAuthenticated(ctx context.Context, sessionID string) {
	dbusMethod := DbusInterface + ".CancelIsAuthenticated"

	call := b.dbusObject.Call(dbusMethod, 0, sessionID)
	if call.Err != nil {
		log.Errorf(ctx, "could not cancel IsAuthenticated call for session %q: %v", sessionID, call.Err)
	}
}
