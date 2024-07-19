//go:build pam_tests_exec_client

package main

import (
	"context"
	"errors"

	"github.com/msteinert/pam/v2"
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
func (m *moduleWrapper) SimulateClientPanic(text string) {
	panic(text)
}

// SimulateClientError forces the client to return a new Go error with no PAM type.
func (m *moduleWrapper) SimulateClientError(errorMsg string) error {
	return errors.New(errorMsg)
}
