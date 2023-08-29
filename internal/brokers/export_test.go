package brokers

import (
	"context"
	"fmt"
	"sort"

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

// GenerateLayoutValidators generates the layout validators and assign them to the specified broker.
func GenerateLayoutValidators(b *Broker, sessionID string, supportedUILayouts []map[string]string) {
	b.layoutValidators = generateValidators(context.Background(), sessionID, supportedUILayouts)
}

// LayoutValidatorsString returns a string representation of the layout validators.
func (b *Broker) LayoutValidatorsString() string {
	// Gets the map keys and sort them
	var keys []string
	for k := range b.layoutValidators {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var s string
	for _, k := range keys {
		layoutStr := fmt.Sprintf("\t%s:\n", k)

		validator := b.layoutValidators[k]

		// Same thing for sorting the keys of the validator map
		var vKeys []string
		for v := range validator {
			vKeys = append(vKeys, v)
		}
		sort.Strings(vKeys)

		for _, v := range vKeys {
			layoutStr += fmt.Sprintf("\t\t%s: { required: %v, supportedValues: %v }\n", v, validator[v].required, validator[v].supportedValues)
		}

		s += layoutStr
	}
	return s
}
