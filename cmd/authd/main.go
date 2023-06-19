// Package main is the windows-agent entry point.
package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ubuntu/authd/cmd/authd/daemon"
	"github.com/ubuntu/authd/internal/log"
)

//go:generate go run ../generate_completion_documentation.go completion ../../generated
//go:generate go run ../generate_completion_documentation.go update-readme
//go:generate go run ../generate_completion_documentation.go update-doc-cli-ref

func main() {
	//i18n.InitI18nDomain(common.TEXTDOMAIN)
	a := daemon.New()
	os.Exit(run(a))
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
