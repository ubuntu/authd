// Package main is the windows-agent entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ubuntu/authd/cmd/authd/daemon"
	"github.com/ubuntu/authd/internal/brokers/examplebroker"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/testutils"
)

//FIXME go:generate go run ../generate_completion_documentation.go completion ../../generated
//FIXME go:generate go run ../generate_completion_documentation.go update-readme
//FIXME go:generate go run ../generate_completion_documentation.go update-doc-cli-ref

func main() {
	//i18n.InitI18nDomain(common.TEXTDOMAIN)
	busCleanup, err := testutils.StartSystemBusMock()
	if err != nil {
		os.Exit(1)
	}
	fmt.Printf("Mock system bus started on %s\n", os.Getenv("DBUS_SYSTEM_BUS_ADDRESS"))

	// Create the directory for the broker configuration files.
	if err = os.MkdirAll("/etc/authd/broker.d", 0750); err != nil {
		busCleanup()
		os.Exit(1)
	}
	cleanup := func() {
		os.RemoveAll("/etc/authd/broker.d")
		busCleanup()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		if err = examplebroker.StartBus(ctx); err != nil {
			log.Error(context.Background(), err)
			cleanup()
			os.Exit(1)
		}
		close(done)
	}()
	// Give some time for the broker to start
	time.Sleep(time.Second)

	exitCode := run(daemon.New())
	// After the daemon has exited, we can stop the broker as well.
	cancel()
	<-done
	// We use os.Exit, so we need to call cleanup manually since defered functions won't be executed.
	cleanup()
	os.Exit(exitCode)
}

type app interface {
	Run() error
	UsageError() bool
	Hup() bool
	Quit()
}

func run(a app) int {
	defer installSignalHandler(a)()

	log.SetFormatter(&log.TextFormatter{
		DisableLevelTruncation: true,
		DisableTimestamp:       true,

		// ForceColors is necessary on Windows, not only to have colors but to
		// prevent logrus from falling back to structured logs.
		ForceColors: true,
	})

	if err := a.Run(); err != nil {
		log.Error(context.Background(), err)

		if a.UsageError() {
			return 2
		}
		return 1
	}

	return 0
}

func installSignalHandler(a app) func() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			switch v, ok := <-c; v {
			case syscall.SIGINT, syscall.SIGTERM:
				a.Quit()
				return
			case syscall.SIGHUP:
				if a.Hup() {
					a.Quit()
					return
				}
			default:
				// channel was closed: we exited
				if !ok {
					return
				}
			}
		}
	}()

	return func() {
		signal.Stop(c)
		close(c)
		wg.Wait()
	}
}
