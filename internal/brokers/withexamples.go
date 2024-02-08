//go:build withexamplebroker

package brokers

import (
	"context"
	"os"
	"time"

	"github.com/ubuntu/authd/examplebroker"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/testutils"
)

// useExampleBrokers starts a mock system bus and exports the examplebroker in it.
func useExampleBrokers() (string, func(), error) {
	busCleanup, err := testutils.StartSystemBusMock()
	if err != nil {
		return "", nil, err
	}
	log.Debugf(context.Background(), "Mock system bus started on %s\n", os.Getenv("DBUS_SYSTEM_BUS_ADDRESS"))

	// Create the directory for the broker configuration files.
	cfgPath, err := os.MkdirTemp(os.TempDir(), "examplebroker.d")
	if err != nil {
		busCleanup()
		return "", nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer os.RemoveAll(cfgPath)
		defer busCleanup()
		if err = examplebroker.StartBus(ctx, cfgPath); err != nil {
			log.Errorf(ctx, "Error starting examplebroker: %v", err)
		}
	}()

	// Give some time for the broker to start
	time.Sleep(time.Second)

	return cfgPath, func() {
		cancel()
		<-done
	}, nil
}

// Stop calls the function responsible for cleaning up the examplebrokers.
func (m *Manager) Stop() {
	m.cleanup()
}
