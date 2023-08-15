package brokers

import (
	"context"

	"github.com/godbus/dbus/v5"
)

// NewBroker exports the private newBroker function for testing purposes.
func NewBroker(ctx context.Context, name, configFile string, bus *dbus.Conn) (Broker, error) {
	return newBroker(ctx, name, configFile, bus)
}

// WithCfgDir uses a dedicated path for the broker config dir.
func WithCfgDir(p string) func(o *options) {
	return func(o *options) {
		o.brokerCfgDir = p
	}
}

// SetBrokerForSession sets the broker for a given session.
//
// This is to be used only in tests.
func (m *Manager) SetBrokerForSession(b *Broker, sessionID string) {
	m.transactionsToBrokerMu.Lock()
	m.transactionsToBroker[sessionID] = b
	m.transactionsToBrokerMu.Unlock()
}
