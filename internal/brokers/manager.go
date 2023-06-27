package brokers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/godbus/dbus/v5"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/decorate"
)

type Manager struct {
	transactions map[string]*Broker
	brokers      []Broker
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

func NewManager(ctx context.Context, configuredBrokers []string, args ...Option) (m Manager, err error) {
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

	// Load brokers configuration
	for _, n := range configuredBrokers {
		configFile := filepath.Join(brokersConfPath, n)
		b, err := NewBroker(ctx, n, configFile, bus)
		if err != nil {
			log.Errorf(ctx, "Skipping broker %q is not correctly configured: %v", n, err)
			continue
		}
		m.brokers = append(m.brokers, b)
	}

	return m, nil
}

// AvailableBrokers returns currenctly loaded and available brokers.
func (m Manager) AvailableBrokers() (r []Broker) {
	return m.brokers
}
