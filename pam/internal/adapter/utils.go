package adapter

import (
	"errors"
	"os"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/msteinert/pam/v2"
)

var (
	isSSHSession     bool
	isSSHSessionOnce sync.Once
)

// convertTo converts an interface I value to T. It will panic (progamming error) if this is not the case.
func convertTo[T any, I any](elem I) T {
	//nolint:forcetypeassert // if the conversion do not pass, this is a programmer error. Assert it hard.
	return any(elem).(T)
}

// TeaHeadlessOptions gets the options to run a bubbletea program in headless mode.
func TeaHeadlessOptions() ([]tea.ProgramOption, error) {
	// Explicitly set the output to something so that the program
	// won't try to init some terminal fancy things that also appear
	// to be racy...
	// See: https://github.com/charmbracelet/bubbletea/issues/910
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, errors.Join(err, pam.ErrSystem)
	}
	return []tea.ProgramOption{
		tea.WithInput(nil),
		tea.WithoutRenderer(),
		tea.WithoutSignals(),
		tea.WithoutSignalHandler(),
		tea.WithoutCatchPanics(),
		tea.WithOutput(devNull),
	}, nil
}

func isSSHSessionFunc(mTx pam.ModuleTransaction) bool {
	service, _ := mTx.GetItem(pam.Service)
	if service == "sshd" {
		return true
	}

	envs, err := mTx.GetEnvList()
	if err != nil {
		return false
	}
	if _, ok := envs["SSH_CONNECTION"]; ok {
		return true
	}
	if _, ok := envs["SSH_AUTH_INFO_0"]; ok {
		return true
	}
	return false
}

// IsSSHSession checks if the module transaction is currently handling a SSH session.
func IsSSHSession(mTx pam.ModuleTransaction) bool {
	isSSHSessionOnce.Do(func() { isSSHSession = isSSHSessionFunc(mTx) })
	return isSSHSession
}

func maybeSendPamError(err error) tea.Cmd {
	if err == nil {
		return nil
	}

	var errPam pam.Error
	if errors.As(err, &errPam) {
		return sendEvent(pamError{status: errPam, msg: err.Error()})
	}
	return sendEvent(pamError{status: pam.ErrSystem, msg: err.Error()})
}
