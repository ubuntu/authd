package brokers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/decorate"
)

type Manager struct {
	brokers      map[string]*Broker
	brokersOrder []string

	usersToBroker   map[string]*Broker
	usersToBrokerMu sync.RWMutex

	transactionsToBroker   map[string]*Broker
	transactionsToBrokerMu sync.RWMutex
}

// Option is the function signature used to tweak the daemon creation.
type Option func(*options)

type options struct {
	rootDir string
}

// WithRootDir uses a dedicated path for our root.
func WithRootDir(p string) func(o *options) {
	return func(o *options) {
		o.rootDir = p
	}
}

func NewManager(ctx context.Context, configuredBrokers []string, args ...Option) (m *Manager, err error) {
	defer decorate.OnError(&err /*i18n.G(*/, "can't create brokers detection object") //)

	log.Debug(ctx, "Building broker detection")

	// Set default options.
	opts := options{
		rootDir: "/",
	}
	// Apply given args.
	for _, f := range args {
		f(&opts)

	}

	// Connecr to to system bus
	// Don't call dbus.SystemBus which caches globally system dbus (issues in tests)
	bus, err := dbus.SystemBusPrivate()
	if err != nil {
		return m, err
	}
	if err = bus.Auth(nil); err != nil {
		_ = bus.Close()
		return m, err
	}
	if err = bus.Hello(); err != nil {
		_ = bus.Close()
		return m, err
	}

	// FIXME: what to do without brokers? and without those directories?

	brokersConfPath := filepath.Join(opts.rootDir, "etc/authd/broker.d")

	// Select all brokers in ascii order if none is configured
	if len(configuredBrokers) == 0 {
		log.Debug(ctx, "Auto-detecting brokers")

		entries, err := os.ReadDir(brokersConfPath)
		if err != nil {
			return m, fmt.Errorf("could not read brokers directory to detect brokers: %v", err)
		}
		for _, e := range entries {
			if !e.Type().IsRegular() {
				continue
			}
			configuredBrokers = append(configuredBrokers, e.Name())
		}
	}

	brokers := make(map[string]*Broker)
	var brokersOrder []string

	// First broker is always the local one.
	b, err := NewBroker(ctx, localBrokerName, "", nil)
	brokersOrder = append(brokersOrder, b.ID)
	brokers[b.ID] = &b

	// Load brokers configuration
	for _, n := range configuredBrokers {
		configFile := filepath.Join(brokersConfPath, n)
		b, err := NewBroker(ctx, n, configFile, bus)
		if err != nil {
			log.Errorf(ctx, "Skipping broker %q is not correctly configured: %v", n, err)
			continue
		}
		brokersOrder = append(brokersOrder, b.ID)
		brokers[b.ID] = &b
	}

	// Add example brokers
	for _, n := range []string{"broker foo", "broker bar"} {
		b, err := NewBroker(ctx, n, "", nil)
		if err != nil {
			log.Errorf(ctx, "Skipping broker %q is not correctly configured: %v", n, err)
			continue
		}
		brokersOrder = append(brokersOrder, b.ID)
		brokers[b.ID] = &b
	}

	return &Manager{
		brokers:      brokers,
		brokersOrder: brokersOrder,

		usersToBroker:        make(map[string]*Broker),
		transactionsToBroker: make(map[string]*Broker),
	}, nil
}

// AvailableBrokers returns currently loaded and available brokers in preference order.
func (m *Manager) AvailableBrokers() (r []*Broker) {
	for _, id := range m.brokersOrder {
		r = append(r, m.brokers[id])
	}
	return r
}

// GetBroker returns the broker matching this brokerID.
func (m *Manager) GetBroker(brokerID string) (broker *Broker, err error) {
	broker, exists := m.brokers[brokerID]
	if !exists {
		return nil, fmt.Errorf("no broker found matching %q", brokerID)
	}

	return broker, nil
}

// SetDefaultBrokerForUser memorizes which broker was used for which user.
func (m *Manager) SetDefaultBrokerForUser(username string, broker *Broker) {
	m.usersToBrokerMu.Lock()
	defer m.usersToBrokerMu.Unlock()
	m.usersToBroker[username] = broker
}

// BrokerForUser returns any previously selected broker for a given user, if any.
func (m *Manager) BrokerForUser(username string) (broker *Broker) {
	m.usersToBrokerMu.RLock()
	defer m.usersToBrokerMu.RUnlock()
	return m.usersToBroker[username]
}

// SetBrokerForSessionID set a broker as currently use for a given transaction with this sessionID.
func (m *Manager) SetBrokerForSessionID(sessionID string, broker *Broker) {
	m.transactionsToBrokerMu.Lock()
	defer m.transactionsToBrokerMu.Unlock()
	m.transactionsToBroker[sessionID] = broker
}

// BrokerForSessionID returns broker currently in use for a given transaction sessionID.
func (m *Manager) BrokerForSessionID(sessionID string) (broker *Broker, err error) {
	m.transactionsToBrokerMu.RLock()
	defer m.transactionsToBrokerMu.RUnlock()
	broker, exists := m.transactionsToBroker[sessionID]
	if !exists {
		return nil, fmt.Errorf("no broker found for session %q", sessionID)
	}

	return broker, nil
}
