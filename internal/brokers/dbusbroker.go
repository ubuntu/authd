package brokers

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/decorate"
	"gopkg.in/ini.v1"
)

type dbusBroker struct {
	dbusObject    dbus.BusObject
	interfaceName string
}

// newDbusBroker returns a dbus broker and broker attributes from its configuration file.
func newDbusBroker(ctx context.Context, bus *dbus.Conn, configFile string) (b dbusBroker, fullName, brandIcon string, err error) {
	defer decorate.OnError(&err, "dbus broker from configuration file: %q", configFile)

	log.Debugf(ctx, "Dbus broker configuration at %q", configFile)

	cfg, err := ini.Load(configFile)
	if err != nil {
		return b, "", "", fmt.Errorf("could not read ini configuration for broker %v", err)
	}

	fullNameVal, err := cfg.Section("").GetKey("name")
	if err != nil {
		return b, "", "", fmt.Errorf("missing field for broker: %v", err)
	}

	brandIconVal, err := cfg.Section("").GetKey("brand_icon")
	if err != nil {
		return b, "", "", fmt.Errorf("missing field for broker: %v", err)
	}

	dbusName, err := cfg.Section("dbus").GetKey("name")
	if err != nil {
		return b, "", "", fmt.Errorf("missing field for broker: %v", err)
	}

	objectName, err := cfg.Section("dbus").GetKey("object")
	if err != nil {
		return b, "", "", fmt.Errorf("missing field for broker: %v", err)
	}

	interfaceName, err := cfg.Section("dbus").GetKey("interface")
	if err != nil {
		return b, "", "", fmt.Errorf("missing field for broker: %v", err)
	}

	return dbusBroker{
		dbusObject:    bus.Object(dbusName.String(), dbus.ObjectPath(objectName.String())),
		interfaceName: interfaceName.String(),
	}, fullNameVal.String(), brandIconVal.String(), nil
}

// To be implemented.
func (b dbusBroker) GetAuthenticationModes(ctx context.Context, username, lang string, supportedUILayouts []map[string]string) (sessionID, encryptionKey string, authenticationModes []map[string]string, err error) {
	return "", "", nil, nil
}
func (b dbusBroker) SelectAuthenticationMode(ctx context.Context, sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, err error) {
	return nil, nil
}
func (b dbusBroker) IsAuthorized(ctx context.Context, sessionID, authenticationData string) (access, infoUser string, err error) {
	return "", "", nil
}
func (b dbusBroker) AbortSession(ctx context.Context, sessionID string) (err error) {
	return nil
}
func (b dbusBroker) CancelIsAuthorized(ctx context.Context, sessionID string) {
}
