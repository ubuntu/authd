package examplebroker

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/ubuntu/decorate"
)

const (
	dbusObjectPath = "/com/ubuntu/auth/ExampleBroker"
	dbusInterface  = "com.ubuntu.auth.ExampleBroker"
)

// Bus is the D-Bus object that will answer calls for the broker.
type Bus struct {
	broker *broker
}

// StartBus starts the D-Bus service and exports it on the system bus.
func StartBus() (err error) {
	defer decorate.OnError(&err, "could not start example broker bus")

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return err
	}
	defer conn.Close()

	b, _, _ := newBroker("ExampleBroker")
	obj := Bus{broker: b}
	err = conn.Export(&obj, dbusObjectPath, dbusInterface)
	if err != nil {
		_ = conn.Close()
		return err
	}

	if err = conn.Export(introspect.NewIntrospectable(&introspect.Node{
		Name: dbusObjectPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{
				Name:    dbusInterface,
				Methods: introspect.Methods(&obj),
			},
		},
	}), dbusObjectPath, introspect.IntrospectData.Name); err != nil {
		_ = conn.Close()
		return err
	}

	reply, err := conn.RequestName(dbusInterface, dbus.NameFlagDoNotQueue)
	if err != nil {
		_ = conn.Close()
		return err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		_ = conn.Close()
		return fmt.Errorf("D-Bus name already taken")
	}

	select {}
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

// IsAuthorized is the method through which the broker and the daemon will communicate once dbusInterface.IsAuthorized is called.
func (b *Bus) IsAuthorized(sessionID, authenticationData string) (access, infoUser string, dbusErr *dbus.Error) {
	access, infoUser, err := b.broker.IsAuthorized(context.Background(), sessionID, authenticationData)
	if err != nil {
		return "", "", dbus.MakeFailedError(err)
	}
	return access, infoUser, nil
}

// EndSession is the method through which the broker and the daemon will communicate once dbusInterface.EndSession is called.
func (b *Bus) EndSession(sessionID string) (dbusErr *dbus.Error) {
	err := b.broker.EndSession(context.Background(), sessionID)
	if err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

// CancelIsAuthorized is the method through which the broker and the daemon will communicate once dbusInterface.CancelIsAuthorized is called.
func (b *Bus) CancelIsAuthorized(sessionID string) (dbusErr *dbus.Error) {
	b.broker.CancelIsAuthorized(context.Background(), sessionID)
	return nil
}
