//go:build withexamplebroker

package brokers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ubuntu/authd/internal/brokers/examplebroker"
	"github.com/ubuntu/authd/internal/log"
)

// useExampleBrokers starts a mock system bus and exports the examplebroker in it.
func useExampleBrokers() (string, func(), error) {
	busCleanup, err := startSystemBusMock()
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

// startSystemBusMock starts a mock dbus daemon and returns a cancel function to stop it.
func startSystemBusMock() (func(), error) {
	busDir := filepath.Join(os.TempDir(), "authd-system-bus-mock")
	if err := os.MkdirAll(busDir, 0750); err != nil {
		return nil, err
	}

	cfgPath := filepath.Join(busDir, "bus.conf")
	listenPath := filepath.Join(busDir, "bus.sock")

	err := os.WriteFile(cfgPath, []byte(fmt.Sprintf(localSystemBusCfg, listenPath)), 0600)
	if err != nil {
		err = errors.Join(err, os.RemoveAll(busDir))
		return nil, err
	}

	busCtx, busCancel := context.WithCancel(context.Background())
	//#nosec:G204 // This is only for manual testing purposes and won't be in production code.
	cmd := exec.CommandContext(busCtx, "dbus-daemon", "--config-file="+cfgPath)
	if err := cmd.Start(); err != nil {
		busCancel()
		err = errors.Join(err, os.RemoveAll(busDir))
		return nil, err
	}
	// Give some time for the daemon to start.
	time.Sleep(500 * time.Millisecond)

	prev, set := os.LookupEnv("DBUS_SYSTEM_BUS_ADDRESS")
	os.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path="+listenPath)

	return func() {
		busCancel()
		_ = cmd.Wait()
		_ = os.RemoveAll(busDir)

		if !set {
			os.Unsetenv("DBUS_SYSTEM_BUS_ADDRESS")
		} else {
			os.Setenv("DBUS_SYSTEM_BUS_ADDRESS", prev)
		}
	}, nil
}

// Stop calls the function responsible for cleaning up the examplebrokers.
func (m *Manager) Stop() {
	m.cleanup()
}

var localSystemBusCfg = `<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>system</type>
  <keep_umask/>
  <listen>unix:path=%s</listen>
  <policy context="default">
    <allow user="*"/>
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`
