package examplebroker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/decorate"
)

const (
	dbusObjectPath = "/com/ubuntu/authd/ExampleBroker"
	busName        = "com.ubuntu.authd.ExampleBroker"
)

// Bus is the D-Bus object that will answer calls for the broker.
type Bus struct {
	broker *Broker
}

// StartBus starts the D-Bus service and exports it on the system bus.
func StartBus(ctx context.Context, cfgPath string) (err error) {
	defer decorate.OnError(&err, "could not start example broker bus")

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return err
	}
	defer conn.Close()

	b, _, _ := New("ExampleBroker")
	obj := Bus{broker: b}
	err = conn.Export(&obj, dbusObjectPath, brokers.DbusInterface)
	if err != nil {
		return err
	}

	if err = conn.Export(introspect.NewIntrospectable(&introspect.Node{
		Name: dbusObjectPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{
				Name:    brokers.DbusInterface,
				Methods: introspect.Methods(&obj),
			},
		},
	}), dbusObjectPath, introspect.IntrospectData.Name); err != nil {
		return err
	}

	reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("D-Bus name already taken")
	}

	if err = os.WriteFile(filepath.Join(cfgPath, "examplebroker.conf"),
		[]byte(fmt.Sprintf(`
name = ExampleBroker
brand_icon = /usr/share/backgrounds/warty-final-ubuntu.png

[dbus]
name = %s
object = %s
`, busName, dbusObjectPath)),
		0600); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}

// NewSession is the method through which the broker and the daemon will communicate once dbusInterface.NewSession is called.
func (b *Bus) NewSession(username, lang string) (sessionID, encryptionKey string, dbusErr *dbus.Error) {
	sessionID, encryptionKey, err := b.broker.NewSession(context.Background(), username, lang)
	if err != nil {
		return "", "", dbus.MakeFailedError(err)
	}
	return sessionID, encryptionKey, nil
}

// GetAuthenticationModes is the method through which the broker and the daemon will communicate once dbusInterface.GetAuthenticationModes is called.
func (b *Bus) GetAuthenticationModes(sessionID string, supportedUILayouts []map[string]string) (authenticationModes []map[string]string, dbusErr *dbus.Error) {
	authenticationModes, err := b.broker.GetAuthenticationModes(context.Background(), sessionID, supportedUILayouts)
	if err != nil {
		return nil, dbus.MakeFailedError(err)
	}
	return authenticationModes, nil
}

// SelectAuthenticationMode is the method through which the broker and the daemon will communicate once dbusInterface.SelectAuthenticationMode is called.
func (b *Bus) SelectAuthenticationMode(sessionID, authenticationModeName string) (uiLayoutInfo map[string]string, dbusErr *dbus.Error) {
	uiLayoutInfo, err := b.broker.SelectAuthenticationMode(context.Background(), sessionID, authenticationModeName)
	if err != nil {
		return nil, dbus.MakeFailedError(err)
	}
	return uiLayoutInfo, nil
}

// IsAuthenticated is the method through which the broker and the daemon will communicate once dbusInterface.IsAuthenticated is called.
func (b *Bus) IsAuthenticated(sessionID, authenticationData string) (access, data string, dbusErr *dbus.Error) {
	access, data, err := b.broker.IsAuthenticated(context.Background(), sessionID, authenticationData)
	if err != nil {
		return "", "", dbus.MakeFailedError(err)
	}
	return access, data, nil
}

// EndSession is the method through which the broker and the daemon will communicate once dbusInterface.EndSession is called.
func (b *Bus) EndSession(sessionID string) (dbusErr *dbus.Error) {
	err := b.broker.EndSession(context.Background(), sessionID)
	if err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

// CancelIsAuthenticated is the method through which the broker and the daemon will communicate once dbusInterface.CancelIsAuthenticated is called.
func (b *Bus) CancelIsAuthenticated(sessionID string) (dbusErr *dbus.Error) {
	b.broker.CancelIsAuthenticated(context.Background(), sessionID)
	return nil
}
