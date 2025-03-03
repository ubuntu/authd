//go:build pam_tests_exec_client

package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/godbus/dbus/v5"
	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/log"
	"github.com/ubuntu/authd/pam/internal/dbusmodule"
)

type moduleWrapper struct {
	pam.ModuleTransaction
}

func newModuleWrapper(serverAddress string) (pam.ModuleTransaction, func(), error) {
	mTx, closeFunc, err := dbusmodule.NewTransaction(context.TODO(), serverAddress)
	return &moduleWrapper{mTx}, closeFunc, err
}

// SimulateClientPanic forces the client to panic with the provided text.
func (m *moduleWrapper) CallUnhandledMethod() error {
	method := "com.ubuntu.authd.pam.UnhandledMethod"
	tx, _ := m.ModuleTransaction.(*dbusmodule.Transaction)
	return tx.BusObject().Call(method, dbus.FlagNoAutoStart).Err
}

// SimulateClientPanic forces the client to panic with the provided text.
func (m *moduleWrapper) SimulateClientPanic(text string) {
	panic(text)
}

// SimulateClientError forces the client to return a new Go error with no PAM type.
func (m *moduleWrapper) SimulateClientError(errorMsg string) error {
	return errors.New(errorMsg)
}

// SimulateClientSignal sends a signal to the child process.
func (m *moduleWrapper) SimulateClientSignal(sig syscall.Signal) {
	pid := os.Getpid()
	log.Debugf(context.Background(), "Sending signal %v to self pid (%v)",
		sig, pid)

	c := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(c, os.Interrupt)
	go func() {
		for s := range c {
			if s != sig {
				continue
			}
			log.Debugf(context.Background(), "Signal %v received", sig)
			close(done)
			return
		}
	}()

	if err := syscall.Kill(pid, sig); err != nil {
		log.Errorf(context.Background(), "Sending signal %v failed: %v", sig, err)
		return
	}
	<-done
}
