package adapter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/msteinert/pam/v2"
	"github.com/ubuntu/authd/log"
)

var (
	isSSHSessionValue bool
	isSSHSessionOnce  sync.Once

	isTerminalTTYValue bool
	isTerminalTTYOnce  sync.Once
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

// isSSHSession checks if the module transaction is currently handling a SSH session.
func isSSHSession(mTx pam.ModuleTransaction) bool {
	isSSHSessionOnce.Do(func() { isSSHSessionValue = isSSHSessionFunc(mTx) })
	return isSSHSessionValue
}

// GetPamTTY returns the file to that is used by PAM tty or stdin.
func GetPamTTY(mTx pam.ModuleTransaction) (tty *os.File, cleanup func()) {
	var err error
	defer func() {
		if err != nil {
			log.Warningf(context.TODO(), "Failed to open PAM TTY: %s", err)
		}
		if tty == nil {
			tty = os.Stdin
		}
		if cleanup == nil {
			cleanup = func() {}
		}
	}()

	var pamTTY string
	pamTTY, err = mTx.GetItem(pam.Tty)
	if err != nil {
		return nil, nil
	}

	if pamTTY == "" {
		return nil, nil
	}

	tty, err = os.OpenFile(pamTTY, os.O_RDWR, 0600)
	if err != nil {
		return nil, nil
	}
	cleanup = func() { tty.Close() }

	// We check the fd could be passed to x/term to decide if we should fallback to stdin
	if tty.Fd() > math.MaxInt {
		err = fmt.Errorf("unexpected large PAM TTY fd: %d", tty.Fd())
		return nil, cleanup
	}

	return tty, cleanup
}

// IsTerminalTTY returns whether the [pam.Tty] or the [os.Stdin] is a terminal TTY.
func IsTerminalTTY(mTx pam.ModuleTransaction) bool {
	isTerminalTTYOnce.Do(func() {
		tty, cleanup := GetPamTTY(mTx)
		defer cleanup()
		isTerminalTTYValue = term.IsTerminal(tty.Fd())
	})
	return isTerminalTTYValue
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
