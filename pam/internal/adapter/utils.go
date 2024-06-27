package adapter

import (
	"errors"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/msteinert/pam/v2"
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
